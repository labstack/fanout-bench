package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/labstack/fanout-bench/internal/bench"
)

func TestLocalDefaultsUseSharedFanoutListener(t *testing.T) {
	cfg, err := commonFlags("test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Endpoint != "localhost:7520" || cfg.MetricsURL != "http://localhost:7520/metrics" {
		t.Fatalf("endpoint=%q metrics=%q", cfg.Endpoint, cfg.MetricsURL)
	}
}

func TestSplitReproductionCommandPassesValidationAfterSubstitution(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := commonFlags("run", []string{
		"--host", "root@192.0.2.10", "--generator-host", "root@192.0.2.11",
		"--generator-address", "10.77.0.3", "--fanout-listen", "10.77.0.2",
		"--ssh-port", "2222", "--telemetrygen", executable, "--preset", "quick",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Token = "test-token"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := bench.Runner{Config: cfg}
	report, err := runner.Run(ctx)
	if err == nil || report == nil {
		t.Fatalf("expected partial report on cancellation, got report=%v err=%v", report, err)
	}
	command := strings.NewReplacer(
		"<host>", "192.0.2.20", "<generator-host>", "192.0.2.21",
		"<generator-address>", "10.77.0.13", "<fanout-listen>", "10.77.0.12",
	).Replace(report.ReproductionCommand)
	// Parse shell quoting without executing the reproduction command.
	output, err := exec.Command("bash", "-c", "set -- "+command+"; printf '%s\\0' \"$@\"").Output()
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
	reproduced, err := commonFlags("run", args[2:])
	if err != nil {
		t.Fatalf("substituted reproduction failed validation: %v\n%s", err, command)
	}
	if reproduced.Host != "root@192.0.2.20" || reproduced.GeneratorHost != "root@192.0.2.21" ||
		reproduced.GeneratorAddress != "10.77.0.13" || reproduced.FanoutListen != "10.77.0.12" || reproduced.SSHPort != 2222 {
		t.Fatalf("reproduction lost topology: %+v", reproduced)
	}
}

func TestHostFlags(t *testing.T) {
	cfg, err := commonFlags("test", []string{"--ssh-known-hosts", "/tmp/run scope/known_hosts", "--host-description", "Hetzner Cloud ccx33, fsn1, ubuntu-24.04"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SSHKnownHosts != "/tmp/run scope/known_hosts" || cfg.HostDescription != "Hetzner Cloud ccx33, fsn1, ubuntu-24.04" {
		t.Fatalf("known-hosts=%q description=%q", cfg.SSHKnownHosts, cfg.HostDescription)
	}
}

func TestSplitFlagsRequiredTogetherAndRemoteOnly(t *testing.T) {
	flags := [][]string{{"--generator-host", "root@generator.example"}, {"--generator-address", "10.77.0.3"}, {"--fanout-listen", "10.77.0.2"}}
	for mask := 0; mask < 8; mask++ {
		for _, remote := range []bool{false, true} {
			var args []string
			if remote {
				args = append(args, "--host", "root@fanout.example")
			}
			for i, pair := range flags {
				if mask&(1<<i) != 0 {
					args = append(args, pair...)
				}
			}
			cfg, err := commonFlags("run", args)
			valid := mask == 0 || (remote && mask == 7)
			if (err == nil) != valid {
				t.Errorf("args=%q: err=%v, valid=%t", args, err, valid)
			}
			if err == nil && mask == 7 && (cfg.GeneratorHost != flags[0][1] || cfg.GeneratorAddress != flags[1][1] || cfg.FanoutListen != flags[2][1]) {
				t.Errorf("split flags lost: %+v", cfg)
			}
		}
	}
	if _, err := commonFlags("doctor", []string{"--generator-host", "root@generator.example"}); err == nil {
		t.Fatal("doctor accepted a run-only flag")
	}
	if _, err := commonFlags("run", []string{"--generator-host-description", "generator"}); err == nil {
		t.Fatal("local run accepted a generator description")
	}
	args := []string{"--host", "root@fanout.example", "--generator-host-description", "Hetzner ccx43"}
	for _, pair := range flags {
		args = append(args, pair...)
	}
	cfg, err := commonFlags("run", args)
	if err != nil || !strings.Contains(cfg.GeneratorHostDescription, "ccx43") {
		t.Fatalf("generator description=%q, err=%v", cfg.GeneratorHostDescription, err)
	}
}
