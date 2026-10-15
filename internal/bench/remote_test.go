package bench

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchMaturityReleaseVerifiesNamedAsset(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tarball := tar.NewWriter(gz)
	if err := tarball.WriteHeader(&tar.Header{Name: "./fanout", Typeflag: tar.TypeReg, Mode: 0o755, Size: 6}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarball.Write([]byte("binary")); err != nil {
		t.Fatal(err)
	}
	if err := tarball.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(archive.Bytes()))
	const base = "https://github.com/labstack/fanout/releases/download/v2026.10.1-alpha/"
	const asset = "fanout_v2026.10.1-alpha_linux_amd64.tar.gz"
	requests := 0
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	http.DefaultClient = &http.Client{Transport: releaseTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		var body []byte
		switch r.URL.String() {
		case base + "SHA256SUMS":
			body = []byte(digest + "  " + asset + "\n")
		case base + asset:
			body = archive.Bytes()
		default:
			return nil, fmt.Errorf("unexpected release request %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
	path, identity, err := fetchFanout(context.Background(), t.TempDir(), "v2026.10.1-alpha", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "binary" || requests != 2 || identity != base+asset+"#sha256="+digest {
		t.Fatalf("data=%q requests=%d identity=%s", data, requests, identity)
	}
}

func TestSSHTargetValidation(t *testing.T) {
	valid := []string{"root@example.com", "bench-host", "ubuntu@10.0.0.4", "root@[2001:db8::1]"}
	invalid := []string{"-oProxyCommand=bad", "root@host;bad", "root@host name", "root@", ""}
	for _, target := range valid {
		if !sshTargetPattern.MatchString(target) {
			t.Errorf("valid target rejected: %q", target)
		}
	}
	for _, target := range invalid {
		if sshTargetPattern.MatchString(target) {
			t.Errorf("unsafe target accepted: %q", target)
		}
	}
}

func TestFanoutReleaseVersionsAndAssets(t *testing.T) {
	for _, version := range []string{"v2026.10.1-alpha", "v2026.10.2-beta", "v2026.10.3-rc", "v2026.10.4", "v2026.1.0"} {
		got, err := fanoutAsset(version, "linux", "amd64")
		if err != nil {
			t.Fatal(err)
		}
		if want := "fanout_" + version + "_linux_amd64.tar.gz"; got != want {
			t.Errorf("asset=%q want %q", got, want)
		}
	}
	for _, version := range []string{"2026.10.1", "v2026.0.1", "v2026.13.1", "v2026.01.1", "v2026.10.01", "v2026.10.1-alpha.1", "v2026.10.1-preview", "v2026.10.1/evil", "v2026.10.1-rc;bad", ""} {
		if _, err := fanoutAsset(version, "linux", "amd64"); err == nil {
			t.Errorf("accepted invalid release %q", version)
		}
	}
}

func TestParseRemotePlatformIgnoresLoginNoise(t *testing.T) {
	osName, arch, err := parseRemotePlatform("Welcome to the host\n__FANOUT_BENCH_PLATFORM__ Linux aarch64\n")
	if err != nil {
		t.Fatal(err)
	}
	if osName != "linux" || arch != "arm64" {
		t.Fatalf("platform=%s/%s", osName, arch)
	}
}

func TestGoInstallOutputUsesCrossCompileDirectory(t *testing.T) {
	got := goInstallOutput("/tmp/gopath", "linux", "arm64", "darwin", "arm64", "telemetrygen")
	want := "/tmp/gopath/bin/linux_arm64/telemetrygen"
	if got != want {
		t.Fatalf("path=%q want %q", got, want)
	}
}

func TestManagedRemoteCommandQuotesEveryValue(t *testing.T) {
	cfg := Config{Host: "root@192.0.2.10", HostDescription: "Hetzner Cloud ccx33, fsn1, ubuntu-24.04", SSHPort: 2222, SSHIdentity: "/keys/bench key", FanoutVersion: "v2026.10.1-alpha", Preset: "quick", Repeat: 1, BatchSize: 1000, TelemetrygenVersion: "v0.157.0"}
	got := managedRemoteCommand("/tmp/fanout-bench-safe", cfg, "dev", "asset#sha256=abc")
	for _, want := range []string{"'managed'", "'--fanout-version'", "'v2026.10.1-alpha'", "'--provisioned-host'", "'root@<host>'", "'--host-description'", "'Hetzner Cloud ccx33, fsn1, ubuntu-24.04'", "'--provisioned-ssh-port'", "'2222'", "'--provisioned-ssh-identity'", "'/keys/bench key'"} {
		if !strings.Contains(got, want) {
			t.Errorf("command missing %s: %s", want, got)
		}
	}
	if strings.Contains(got, "192.0.2.10") {
		t.Fatal("remote agent does not need the SSH address")
	}
	if strings.Contains(got, "Bearer") {
		t.Fatal("remote command must not carry credentials")
	}
}

func TestSplitManagedRemoteCommand(t *testing.T) {
	cfg := Config{Host: "root@192.0.2.10", SSHPort: 2222, GeneratorHost: "root@192.0.2.11", GeneratorAddress: "10.77.0.3", FanoutListen: "10.77.0.2", GeneratorHostDescription: "Generator's ccx43"}
	got := managedRemoteCommand("/tmp/run", cfg, "dev", "verified")
	for _, want := range []string{
		"chmod 0600 '/tmp/run/generator_key'",
		"'--listen' '10.77.0.2'", "'--generator-ssh' 'root@10.77.0.3'",
		"'--generator-ssh-port' '2222'",
		"'--generator-ssh-identity' '/tmp/run/generator_key'",
		"'--generator-telemetrygen' '/tmp/run-generator/telemetrygen'",
		"'--generator-host-description' 'Generator'\\''s ccx43'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, unwanted := range []string{"192.0.2.10", "192.0.2.11", "'/tmp/run/telemetrygen'", "'--telemetrygen'"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unexpected %q in %s", unwanted, got)
		}
	}
	cfg.GeneratorAddress = "fd77::3"
	if got := managedRemoteCommand("/tmp/run", cfg, "dev", "verified"); !strings.Contains(got, "'root@[fd77::3]'") {
		t.Fatalf("IPv6 target missing brackets: %s", got)
	}
}

func TestGeneratorSSHDConfigIsScopedToRun(t *testing.T) {
	for _, marker := range []string{"fanout-bench-first", "fanout-bench-second"} {
		path := shellQuote("/etc/ssh/sshd_config.d/" + marker + ".conf")
		key := "ssh-ed25519 AAAAtest " + marker
		install := authorizeGeneratorKeyCommand(key+"\n", marker)
		remove := removeGeneratorKeyCommand(marker)
		if !strings.Contains(install, shellQuote(key)+" >> /root/.ssh/authorized_keys") ||
			!strings.Contains(install, " > "+path+" && "+reloadSSHD) {
			t.Fatalf("key and run-specific sshd config not installed together: %s", install)
		}
		if !strings.Contains(remove, shellQuote("/ "+marker+"$/d")) ||
			!strings.Contains(remove, "rm -f "+path+" && "+reloadSSHD) {
			t.Fatalf("cleanup missing run-specific key/config removal and reload: %s", remove)
		}
		for _, command := range []string{install, remove} {
			// Every reference in the drop-in directory must name this run's file.
			withoutPath := strings.ReplaceAll(command, path, "")
			if strings.Contains(withoutPath, "/etc/ssh/sshd_config.d/") {
				t.Fatalf("command touches another sshd drop-in: %s", command)
			}
		}
	}
}

func TestGeneratorHostValidation(t *testing.T) {
	valid := Config{Host: "root@fanout", GeneratorHost: "root@generator", GeneratorAddress: "10.77.0.3", FanoutListen: "10.77.0.2"}
	if err := ValidateGeneratorHosts(valid); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"hostname", "0.0.0.0", "::", "10.77.0.2:7520"} {
		for _, field := range []string{"generator", "fanout"} {
			cfg := valid
			if field == "generator" {
				cfg.GeneratorAddress = value
			} else {
				cfg.FanoutListen = value
			}
			if err := ValidateGeneratorHosts(cfg); err == nil {
				t.Errorf("accepted %s IP %q", field, value)
			}
		}
	}
	for _, value := range []string{"-oProxyCommand=bad", "root@host;bad", "root@host name"} {
		cfg := valid
		cfg.GeneratorHost = value
		if err := ValidateGeneratorHosts(cfg); err == nil {
			t.Errorf("accepted unsafe generator SSH target %q", value)
		}
	}
}

func TestExtractBinarySelectsOnlyNamedRegularFile(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "release.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"docs/README": "ignore", "release/fanout": "binary"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "fanout")
	if err := extractBinary(archivePath, destination, "fanout"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "binary" {
		t.Fatalf("content=%q", data)
	}
}

func TestCompleteSetupCapturesCredentialsWithoutLogging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Fanout-Request") != "1" {
			t.Error("missing request marker")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["setup_token"] != "setup-secret" {
			t.Errorf("setup token=%q", body["setup_token"])
		}
		http.SetCookie(w, &http.Cookie{Name: "fanout_session", Value: "session-secret", Path: "/"})
		_ = json.NewEncoder(w).Encode(map[string]string{"ingest_token": "ingest-secret"})
	}))
	defer server.Close()
	ingest, cookie, err := completeSetup(context.Background(), server.URL, "setup-secret")
	if err != nil {
		t.Fatal(err)
	}
	if ingest != "ingest-secret" || cookie != "fanout_session=session-secret" {
		t.Fatalf("credentials=%q %q", ingest, cookie)
	}
}

func TestSetupTokenPatternCanBeScrubbed(t *testing.T) {
	line := "Open: http://127.0.0.1/login?setup_token=one-time-secret"
	match := setupTokenPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		t.Fatal("token not found")
	}
	scrubbed := strings.ReplaceAll(line, match[1], "[REDACTED]")
	if strings.Contains(scrubbed, "one-time-secret") {
		t.Fatal("token remained in log")
	}
}

// Evidence collection runs after the whole benchmark. Refusing to overwrite an
// existing file there would discard a completed run's report.
func TestCopyEvidenceFileOverwritesPreviousRun(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "fanout.log")
	destination := filepath.Join(dir, "evidence.log")
	if err := os.WriteFile(source, []byte("second run\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("first run\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyEvidenceFile(source, destination); err != nil {
		t.Fatalf("copy: %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second run\n" {
		t.Fatalf("destination=%q", got)
	}
}

func TestKnownHostsReachesSSHAndSCPArgs(t *testing.T) {
	for name, build := range map[string]func(Config) []string{"ssh": sshArgs, "scp": scpArgs} {
		t.Run(name, func(t *testing.T) {
			cfg := Config{SSHPort: 2222, SSHIdentity: "/keys/benchmark key", SSHKnownHosts: "/tmp/run scope/known_hosts"}
			args := build(cfg)
			for _, pair := range [][]string{{"-o", "UserKnownHostsFile=" + cfg.SSHKnownHosts}, {"-i", cfg.SSHIdentity}} {
				index := slices.Index(args, pair[0])
				if pair[0] == "-o" {
					index = slices.Index(args, pair[1]) - 1
				}
				if index < 0 || index+1 >= len(args) || args[index] != pair[0] || args[index+1] != pair[1] {
					t.Fatalf("missing option pair %q in %q", pair, args)
				}
			}
			for _, arg := range build(Config{SSHPort: 22}) {
				if strings.HasPrefix(arg, "UserKnownHostsFile=") {
					t.Fatal("default known-hosts behavior changed")
				}
			}
		})
	}
}
