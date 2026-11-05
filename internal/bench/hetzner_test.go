package bench

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Exercise the real shell entry point in an isolated repo with fake cloud/SSH
// commands. No cloud credentials, shared binaries or actual resources are used.
func TestHetznerParallelLifecycle(t *testing.T) {
	for _, mode := range []string{"success", "failure", "create-failure", "delete-failure", "interrupt", "interrupt-create"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			for _, sub := range []string{"scripts", "bin", "fake", "resources"} {
				if err := os.Mkdir(filepath.Join(dir, sub), 0700); err != nil {
					t.Fatal(err)
				}
			}
			script, err := os.ReadFile("../../scripts/hetzner.sh")
			if err != nil {
				t.Fatal(err)
			}
			write := func(path, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			write("scripts/hetzner.sh", string(script))
			write("fake/hcloud", `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$MOCK_ROOT/events"
kind=$1; action=$2
case "$action" in
 create)
  shift 2
  while [[ $1 != --name ]]; do shift; done
  name=$2
  touch "$MOCK_ROOT/resources/$name"
  if [[ $MOCK_MODE == interrupt-create && $kind == server ]]; then /bin/sleep 0.5; fi
  if [[ $MOCK_MODE == create-failure && $kind == server ]]; then exit 1; fi
  ;;
 delete)
  if [[ $MOCK_MODE == delete-failure && $kind == network ]]; then exit 1; fi
  rm -f "$MOCK_ROOT/resources/$3"
  ;;
 describe) if [[ $kind == location ]]; then echo eu-central; else echo 10.77.0.2; fi ;;
 ip) echo 192.0.2.1 ;;
 list) for f in "$MOCK_ROOT"/resources/*; do [[ -e $f ]] && basename "$f"; done; exit 0 ;;
esac
`)
			write("fake/just", "#!/bin/sh\necho build >> \"$MOCK_ROOT/builds\"\n")
			write("fake/ssh", "#!/bin/sh\nexit 0\n")
			write("fake/sleep", "#!/bin/sh\nexit 0\n")
			write("bin/fanout-bench", `#!/usr/bin/env bash
printf '%s\n' "$*" >> "$MOCK_ROOT/runs"
if [[ $MOCK_MODE == interrupt ]]; then exec /bin/sleep 60; fi
if [[ $MOCK_MODE == success ]]; then
    # Both runs must start before either finishes: sequential execution hangs.
    for ((attempt=0; attempt<500; attempt++)); do
        if [[ $(wc -l < "$MOCK_ROOT/runs") -eq 2 ]]; then exit 0; fi
        /bin/sleep 0.01
    done
    exit 9
fi
if [[ $MOCK_MODE == failure && $* == *fail* ]]; then exit 7; fi
`)
			cmd := exec.Command("bash", filepath.Join(dir, "scripts/hetzner.sh"), "--parallel-groups", "--output", "pass", "--", "--output", "fail", "--signals", "mixed")
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "fake")+":"+os.Getenv("PATH"), "FANOUT_BENCH_HCLOUD_SSH_KEY=test", "FANOUT_BENCH_GROUP_CHILD=", "MOCK_ROOT="+dir, "MOCK_MODE="+mode)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			if mode == "interrupt" || mode == "interrupt-create" {
				deadline := time.Now().Add(10 * time.Second)
				for {
					runs, _ := os.ReadFile(filepath.Join(dir, "runs"))
					events, _ := os.ReadFile(filepath.Join(dir, "events"))
					if (mode == "interrupt" && strings.Count(string(runs), "\n") == 2) || (mode == "interrupt-create" && strings.Count(string(events), "server create") == 2) {
						break
					}
					if time.Now().After(deadline) {
						_ = cmd.Process.Kill()
						t.Fatal("children never reached benchmarks")
					}
					time.Sleep(10 * time.Millisecond)
				}
				if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if (err == nil) != (mode == "success") {
					t.Fatalf("err=%v output=%s", err, output.String())
				}
			case <-time.After(15 * time.Second):
				_ = cmd.Process.Kill()
				t.Fatal("parallel cleanup hung")
			}
			resources, _ := os.ReadDir(filepath.Join(dir, "resources"))
			want := 0
			if mode == "delete-failure" {
				want = 2
			}
			if len(resources) != want {
				t.Fatalf("remaining=%v output=%s", resources, output.String())
			}
			builds, _ := os.ReadFile(filepath.Join(dir, "builds"))
			if string(builds) != "build\n" {
				t.Fatalf("build count: %q", builds)
			}
			if strings.Count(output.String(), "Hetzner pair:") != 2 {
				t.Fatal(output.String())
			}
			if mode == "success" || mode == "failure" {
				runs, _ := os.ReadFile(filepath.Join(dir, "runs"))
				if strings.Count(string(runs), "\n") != 2 || !strings.Contains(string(runs), "--signals mixed") {
					t.Fatal(string(runs))
				}
			}
			if mode == "delete-failure" && !strings.Contains(output.String(), "delete it manually") {
				t.Fatal(output.String())
			}
			// Every created resource was offered for deletion, servers before network.
			events, _ := os.ReadFile(filepath.Join(dir, "events"))
			for _, line := range strings.Split(string(events), "\n") {
				fields := strings.Fields(line)
				if len(fields) < 2 || fields[1] != "create" {
					continue
				}
				for i, f := range fields {
					if f == "--name" {
						deletion := fmt.Sprintf("%s delete %s", fields[0], fields[i+1])
						if fields[0] == "server" {
							network := strings.TrimSuffix(strings.TrimSuffix(fields[i+1], "-fanout"), "-generator")
							if strings.Index(string(events), "network delete "+network) < strings.Index(string(events), deletion) {
								t.Fatal("network deleted before server")
							}
						}
						if !strings.Contains(string(events), deletion) {
							t.Fatalf("missing %s", deletion)
						}
					}
				}
			}
		})
	}
}

func TestHetznerRejectsEmptyParallelGroups(t *testing.T) {
	for _, args := range [][]string{{"--parallel-groups"}, {"--parallel-groups", "--", "--preset", "quick"}, {"--parallel-groups", "--preset", "quick", "--"}} {
		out, err := exec.Command("bash", append([]string{"../../scripts/hetzner.sh"}, args...)...).CombinedOutput()
		if err == nil {
			t.Fatalf("accepted %v: %s", args, out)
		}
	}
}
