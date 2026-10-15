package bench

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var sshTargetPattern = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+@)?(?:[A-Za-z0-9._-]+|\[[0-9A-Fa-f:]+\])$`)
var fanoutVersionPattern = regexp.MustCompile(`^v[0-9]{4}\.(?:[1-9]|1[0-2])\.(?:0|[1-9][0-9]*)(?:-(?:alpha|beta|rc))?$`)

func RunRemote(ctx context.Context, cfg Config, version string, out io.Writer) error {
	if !sshTargetPattern.MatchString(cfg.Host) || strings.HasPrefix(cfg.Host, "-") {
		return fmt.Errorf("unsafe SSH target %q", cfg.Host)
	}
	if cfg.SSHPort < 1 || cfg.SSHPort > 65535 {
		return fmt.Errorf("invalid SSH port %d", cfg.SSHPort)
	}
	if cfg.FanoutVersion == "" || cfg.FanoutVersion == "unknown" {
		return errors.New("--fanout-version is required for a remote run")
	}
	if !fanoutVersionPattern.MatchString(cfg.FanoutVersion) {
		return fmt.Errorf("invalid Fanout release %q: want vYYYY.M.N[-alpha|-beta|-rc]", cfg.FanoutVersion)
	}
	if _, err := Plan(cfg); err != nil {
		return err
	}
	if err := requireCommand("ssh"); err != nil {
		return err
	}
	if err := requireCommand("scp"); err != nil {
		return err
	}
	if err := requireCommand("go"); err != nil {
		return err
	}

	fmt.Fprintf(out, "Fanout Bench remote provision\n  host       %s\n  release    %s\n  preset     %s × %d\n", cfg.Host, cfg.FanoutVersion, cfg.Preset, cfg.Repeat)
	remoteOS, remoteArch, err := detectRemote(ctx, cfg)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  platform   %s/%s\n", remoteOS, remoteArch)
	if remoteOS != "linux" {
		return fmt.Errorf("remote OS %q is not supported; remote benchmark hosts must run Linux", remoteOS)
	}

	buildDir, err := os.MkdirTemp("", "fanout-bench-provision-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildDir)

	fmt.Fprintln(out, "\n  ◌ building architecture-matched benchmark tools")
	harnessPath, err := buildHarness(ctx, buildDir, remoteOS, remoteArch, version)
	if err != nil {
		return fmt.Errorf("build remote harness: %w", err)
	}
	telemetrygenPath, err := buildTelemetrygen(ctx, buildDir, remoteOS, remoteArch, cfg.TelemetrygenVersion)
	if err != nil {
		return fmt.Errorf("build telemetrygen: %w", err)
	}
	fmt.Fprintln(out, "  ◌ downloading and verifying the Fanout release")
	fanoutPath, artifact, err := fetchFanout(ctx, buildDir, cfg.FanoutVersion, remoteOS, remoteArch)
	if err != nil {
		return fmt.Errorf("fetch Fanout release: %w", err)
	}

	suffix, _ := randomHex(4)
	stage := "/tmp/fanout-bench-" + time.Now().UTC().Format("20060102T150405Z") + "-" + suffix
	if err := sshRun(ctx, cfg, out, "install -d -m 0700 "+shellQuote(stage)); err != nil {
		return fmt.Errorf("create remote stage: %w", err)
	}
	cleanup := func() {
		if cfg.KeepRemote {
			fmt.Fprintf(out, "\n  remote run preserved: %s:%s\n", cfg.Host, stage)
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = sshRun(cleanupCtx, cfg, io.Discard, "rm -rf -- "+shellQuote(stage))
	}
	defer cleanup()

	fmt.Fprintln(out, "  ◌ uploading Fanout, telemetrygen, and the benchmark agent")
	if err := scpUpload(ctx, cfg, stage, harnessPath, telemetrygenPath, fanoutPath); err != nil {
		return err
	}
	remoteCommand := managedRemoteCommand(stage, cfg, version, artifact)
	fmt.Fprint(out, "  ◆ running the complete suite on the remote host\n\n")
	runErr := sshRun(ctx, cfg, out, remoteCommand)

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return err
	}
	fmt.Fprintln(out, "\n  ◌ downloading evidence artifacts")
	downloadErr := scpDownload(ctx, cfg, stage+"/results/.", cfg.OutputDir)
	if runErr != nil {
		return fmt.Errorf("remote benchmark failed: %w", runErr)
	}
	if downloadErr != nil {
		return fmt.Errorf("download artifacts: %w", downloadErr)
	}
	fmt.Fprintf(out, "\nArtifacts downloaded to %s\n", cfg.OutputDir)
	return nil
}

func detectRemote(ctx context.Context, cfg Config) (string, string, error) {
	output, err := sshOutput(ctx, cfg, `printf '__FANOUT_BENCH_PLATFORM__ %s %s\n' "$(uname -s)" "$(uname -m)"`)
	if err != nil {
		return "", "", fmt.Errorf("inspect remote host: %w", err)
	}
	return parseRemotePlatform(output)
}

func parseRemotePlatform(output string) (string, string, error) {
	const marker = "__FANOUT_BENCH_PLATFORM__"
	var fields []string
	for _, line := range strings.Split(output, "\n") {
		candidate := strings.Fields(line)
		if len(candidate) == 3 && candidate[0] == marker {
			fields = candidate
			break
		}
	}
	if fields == nil {
		return "", "", fmt.Errorf("remote platform marker missing from %q", output)
	}
	osName := strings.ToLower(fields[1])
	arch := strings.ToLower(fields[2])
	switch arch {
	case "x86_64", "amd64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported remote architecture %q", arch)
	}
	return osName, arch, nil
}

func buildHarness(ctx context.Context, dir, goos, goarch, version string) (string, error) {
	root, err := moduleRoot()
	if err != nil {
		return "", err
	}
	output := filepath.Join(dir, "fanout-bench")
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+version, "-o", output, "./cmd/fanout-bench")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	if data, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w: %s", err, data)
	}
	return output, nil
}

func buildTelemetrygen(ctx context.Context, dir, goos, goarch, version string) (string, error) {
	if version == "" {
		version = "v0.157.0"
	}
	gopath := filepath.Join(dir, "gopath")
	cmd := exec.CommandContext(ctx, "go", "install", "github.com/open-telemetry/opentelemetry-collector-contrib/cmd/telemetrygen@"+version)
	cmd.Env = append(os.Environ(), "GOPATH="+gopath, "GOBIN=", "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	if data, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w: %s", err, data)
	}
	installed := goInstallOutput(gopath, goos, goarch, runtime.GOOS, runtime.GOARCH, "telemetrygen")
	output := filepath.Join(dir, "telemetrygen")
	if err := os.Rename(installed, output); err != nil {
		return "", fmt.Errorf("collect telemetrygen binary: %w", err)
	}
	return output, nil
}

func goInstallOutput(gopath, targetOS, targetArch, hostOS, hostArch, name string) string {
	binDir := filepath.Join(gopath, "bin")
	if targetOS != hostOS || targetArch != hostArch {
		binDir = filepath.Join(binDir, targetOS+"_"+targetArch)
	}
	return filepath.Join(binDir, name)
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if executable, err := os.Executable(); err == nil {
		dir = filepath.Dir(executable)
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("cannot find fanout-bench go.mod; run remote provisioning from a source checkout")
}

func fetchFanout(ctx context.Context, dir, version, goos, goarch string) (string, string, error) {
	asset, err := fanoutAsset(version, goos, goarch)
	if err != nil {
		return "", "", err
	}
	base := "https://github.com/labstack/fanout/releases/download/" + version + "/"
	sumsURL := base + "SHA256SUMS"
	assetURL := base + asset
	sums, err := downloadSmall(ctx, sumsURL, 1<<20)
	if err != nil {
		return "", "", err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if len(want) != 64 {
		return "", "", fmt.Errorf("%s has no checksum for %s", sumsURL, asset)
	}
	archivePath := filepath.Join(dir, asset)
	archive, err := os.Create(archivePath)
	if err != nil {
		return "", "", err
	}
	hash := sha256.New()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		archive.Close()
		return "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		archive.Close()
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		archive.Close()
		return "", "", fmt.Errorf("download %s: status %d", assetURL, resp.StatusCode)
	}
	_, copyErr := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(resp.Body, 1<<30))
	closeErr := archive.Close()
	resp.Body.Close()
	if copyErr != nil {
		return "", "", copyErr
	}
	if closeErr != nil {
		return "", "", closeErr
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if got != want {
		return "", "", fmt.Errorf("checksum mismatch for %s: got %s want %s", asset, got, want)
	}
	binaryPath := filepath.Join(dir, "fanout")
	if err := extractBinary(archivePath, binaryPath, "fanout"); err != nil {
		return "", "", err
	}
	return binaryPath, assetURL + "#sha256=" + got, nil
}

func fanoutAsset(version, goos, goarch string) (string, error) {
	if !fanoutVersionPattern.MatchString(version) {
		return "", fmt.Errorf("invalid Fanout release %q", version)
	}
	if goos != "linux" || (goarch != "amd64" && goarch != "arm64") {
		return "", fmt.Errorf("unsupported Fanout platform %s/%s", goos, goarch)
	}
	return "fanout_" + version + "_" + goos + "_" + goarch + ".tar.gz", nil
}

func downloadSmall(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func extractBinary(archivePath, destination, name string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != name {
			continue
		}
		out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(tr, 512<<20))
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return nil
	}
	return fmt.Errorf("%s not found in %s", name, archivePath)
}

func managedRemoteCommand(stage string, cfg Config, version, artifact string) string {
	args := []string{stage + "/fanout-bench", "managed", "--fanout", stage + "/fanout", "--telemetrygen", stage + "/telemetrygen", "--output", stage + "/results", "--fanout-version", cfg.FanoutVersion, "--fanout-artifact", artifact, "--preset", cfg.Preset, "--repeat", fmt.Sprint(cfg.Repeat), "--batch-size", fmt.Sprint(cfg.BatchSize), "--query-rate", fmt.Sprint(cfg.QueryRate), "--sample-interval", cfg.SampleInterval.String(), "--cooldown", cfg.Cooldown.String(), "--harness-version", version}
	args = append(args, "--telemetrygen-version", cfg.TelemetrygenVersion)
	args = append(args, "--provisioned-host", redactedHost(cfg.Host), "--host-description", cfg.HostDescription)
	args = append(args, "--provisioned-ssh-port", fmt.Sprint(cfg.SSHPort), "--provisioned-ssh-identity", cfg.SSHIdentity)
	args = append(args, "--processes", fmt.Sprint(cfg.ProcessOverride), "--workers", fmt.Sprint(cfg.WorkerOverride), "--rate", fmt.Sprint(cfg.RateOverride), "--duration", cfg.DurationOverride.String())
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return "chmod 0700 " + shellQuote(stage+"/fanout-bench") + " " + shellQuote(stage+"/telemetrygen") + " " + shellQuote(stage+"/fanout") + " && " + strings.Join(quoted, " ")
}

func sshArgs(cfg Config) []string {
	args := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-p", fmt.Sprint(cfg.SSHPort)}
	if cfg.SSHIdentity != "" {
		args = append(args, "-i", cfg.SSHIdentity)
	}
	if cfg.SSHKnownHosts != "" {
		args = append(args, "-o", "UserKnownHostsFile="+cfg.SSHKnownHosts)
	}
	return args
}
func scpArgs(cfg Config) []string {
	args := []string{"-q", "-P", fmt.Sprint(cfg.SSHPort)}
	if cfg.SSHIdentity != "" {
		args = append(args, "-i", cfg.SSHIdentity)
	}
	if cfg.SSHKnownHosts != "" {
		args = append(args, "-o", "UserKnownHostsFile="+cfg.SSHKnownHosts)
	}
	return args
}
func sshRun(ctx context.Context, cfg Config, out io.Writer, command string) error {
	args := append(sshArgs(cfg), cfg.Host, command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}
func sshOutput(ctx context.Context, cfg Config, command string) (string, error) {
	args := append(sshArgs(cfg), cfg.Host, command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(data), nil
}
func scpUpload(ctx context.Context, cfg Config, stage string, paths ...string) error {
	args := append(scpArgs(cfg), paths...)
	args = append(args, cfg.Host+":"+stage+"/")
	cmd := exec.CommandContext(ctx, "scp", args...)
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("upload: %w: %s", err, data)
	}
	return nil
}
func scpDownload(ctx context.Context, cfg Config, remote, destination string) error {
	args := append(scpArgs(cfg), "-r", cfg.Host+":"+remote, destination+"/")
	cmd := exec.CommandContext(ctx, "scp", args...)
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("scp: %w: %s", err, data)
	}
	return nil
}
func requireCommand(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("required command %s: %w", name, err)
	}
	return nil
}
