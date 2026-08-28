package bench

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	cfg := Config{Host: "root@host", SSHPort: 2222, SSHIdentity: "/keys/bench key", FanoutVersion: "v2026.8.11", Preset: "quick", Repeat: 1, BatchSize: 1000, TelemetrygenVersion: "v0.157.0"}
	got := managedRemoteCommand("/tmp/fanout-bench-safe", cfg, "dev", "asset#sha256=abc")
	for _, want := range []string{"'managed'", "'--fanout-version'", "'v2026.8.11'", "'--provisioned-host'", "'root@host'", "'--provisioned-ssh-port'", "'2222'", "'--provisioned-ssh-identity'", "'/keys/bench key'"} {
		if !strings.Contains(got, want) {
			t.Errorf("command missing %s: %s", want, got)
		}
	}
	if strings.Contains(got, "Bearer") {
		t.Fatal("remote command must not carry credentials")
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
