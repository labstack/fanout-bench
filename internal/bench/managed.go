package bench

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ManagedConfig struct {
	HostDescription          string
	GeneratorHostDescription string
	Listen                   string
	GeneratorSSH             string
	GeneratorSSHPort         int
	GeneratorSSHIdentity     string
	GeneratorTelemetrygen    string
	FanoutPath               string
	TelemetrygenPath         string
	OutputDir                string
	FanoutVersion            string
	FanoutArtifact           string
	Preset                   string
	Repeat                   int
	BatchSize                int
	QueryRate                float64
	SampleInterval           time.Duration
	Cooldown                 time.Duration
	HarnessVersion           string
	TelemetrygenVersion      string
	ProvisionedHost          string
	ProvisionedSSHPort       int
	ProvisionedSSHIdentity   string
	ProcessOverride          int
	WorkerOverride           int
	RateOverride             float64
	DurationOverride         time.Duration
}

var setupTokenPattern = regexp.MustCompile(`setup_token=([^\s]+)`)

func RunManaged(ctx context.Context, managed ManagedConfig, out io.Writer) error {
	if managed.GeneratorSSH != "" || managed.GeneratorSSHIdentity != "" || managed.GeneratorTelemetrygen != "" {
		if managed.GeneratorSSH == "" || managed.GeneratorSSHIdentity == "" || managed.GeneratorTelemetrygen == "" {
			return errors.New("--generator-ssh, --generator-ssh-identity, and --generator-telemetrygen are required together")
		}
		if !sshTargetPattern.MatchString(managed.GeneratorSSH) || strings.HasPrefix(managed.GeneratorSSH, "-") {
			return errors.New("invalid generator SSH target")
		}
		managed.TelemetrygenPath = managed.GeneratorTelemetrygen
	}
	if managed.FanoutPath == "" || managed.TelemetrygenPath == "" || managed.OutputDir == "" {
		return errors.New("managed paths are required")
	}
	// Clean first: without it "./results" and "./results/" resolve to different
	// stage directories, so a trailing slash would silently move the Fanout data
	// directory and its log somewhere else.
	stage := filepath.Dir(filepath.Clean(managed.OutputDir))
	dataDir := filepath.Join(stage, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	httpAddr, err := managedListenAddress(managed.Listen)
	if err != nil {
		return err
	}
	authSecret, err := randomHex(32)
	if err != nil {
		return err
	}
	metricsToken, err := randomHex(32)
	if err != nil {
		return err
	}
	grpcAddr := httpAddr
	metricsURL := "http://" + httpAddr + "/metrics"
	baseURL := "http://" + httpAddr
	env := fanoutEnvironment(os.Environ(), dataDir, httpAddr, authSecret, metricsToken)

	actualVersion := strings.TrimSpace(commandOutput(managed.FanoutPath, "version"))
	if actualVersion == "" {
		actualVersion = managed.FanoutVersion
	}
	fmt.Fprintf(out, "Provisioned Fanout %s\n  data       %s\n  OTLP/gRPC  %s\n  metrics    %s\n", actualVersion, dataDir, grpcAddr, metricsURL)

	logPath := filepath.Join(stage, "fanout.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(managed.FanoutPath)
	cmd.Env = env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logFile.Close()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logFile.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	setupTokens := make(chan string, 1)
	processDone := make(chan struct{})
	var processErr error
	var scanWG sync.WaitGroup
	var logMu sync.Mutex
	scan := func(reader io.Reader) {
		defer scanWG.Done()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64*1024), 2<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if match := setupTokenPattern.FindStringSubmatch(line); len(match) == 2 {
				if decoded, decodeErr := url.QueryUnescape(match[1]); decodeErr == nil {
					select {
					case setupTokens <- decoded:
					default:
					}
				}
				line = strings.ReplaceAll(line, match[1], "[REDACTED]")
			}
			logMu.Lock()
			fmt.Fprintln(logFile, redact(line, authSecret, metricsToken))
			logMu.Unlock()
		}
	}
	scanWG.Add(2)
	go scan(stdout)
	go scan(stderr)
	// Drain both pipes before Wait: Wait closes them on process exit, so waiting
	// first truncates the log exactly when Fanout died and the log is the only
	// evidence of why.
	go func() { scanWG.Wait(); processErr = cmd.Wait(); logFile.Close(); close(processDone) }()
	var stopOnce sync.Once
	stopFanout := func() {
		stopOnce.Do(func() {
			if cmd.Process == nil {
				return
			}
			_ = cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-processDone:
			case <-time.After(30 * time.Second):
				_ = cmd.Process.Kill()
				<-processDone
			}
		})
	}
	defer stopFanout()

	var setupToken string
	select {
	case setupToken = <-setupTokens:
	case <-processDone:
		return fmt.Errorf("Fanout exited before setup: %w", processErr)
	case <-time.After(30 * time.Second):
		return errors.New("timed out waiting for Fanout setup token")
	case <-ctx.Done():
		return ctx.Err()
	}
	ingestToken, queryCookie, err := completeSetup(ctx, baseURL, setupToken)
	if err != nil {
		return err
	}

	cfg := managedRunnerConfig(managed, httpAddr, dataDir)
	cfg.FanoutVersion = actualVersion
	cfg.Token, cfg.MetricsToken, cfg.QueryCookie = ingestToken, metricsToken, queryCookie
	runner := Runner{Config: cfg, Version: managed.HarnessVersion, Out: out, DeferSummary: true}
	report, runErr := runner.Run(ctx)
	stopFanout()
	if runErr != nil {
		if report != nil {
			if mkErr := os.MkdirAll(managed.OutputDir, 0o755); mkErr == nil {
				if writeErr := WriteArtifacts(managed.OutputDir, *report); writeErr == nil {
					fmt.Fprintf(out, "\nPartial evidence written to %s\n", managed.OutputDir)
				}
			}
			_ = copyEvidenceFile(logPath, filepath.Join(managed.OutputDir, "fanout.log"))
		}
		return runErr
	}
	verify := exec.CommandContext(ctx, managed.FanoutPath, "repair", "verify")
	verify.Env = env
	verifyOutput, verifyErr := verify.CombinedOutput()
	verifyDetail := "authoritative Parquet storage verified"
	if verifyErr != nil {
		verifyDetail = redact(strings.TrimSpace(string(verifyOutput)), ingestToken, setupToken, queryCookie, authSecret, metricsToken)
		if verifyDetail == "" {
			verifyDetail = verifyErr.Error()
		}
	}
	report.Summary.Gates = append(report.Summary.Gates, Gate{Name: "storage-repair-verify", Passed: verifyErr == nil, Detail: verifyDetail})
	if verifyErr != nil {
		report.Summary.Passed = false
	}
	printSummary(out, *report)
	if err := os.MkdirAll(managed.OutputDir, 0o755); err != nil {
		return err
	}
	repairEvidence := strings.TrimSpace(redact(string(verifyOutput), ingestToken, setupToken, queryCookie, authSecret, metricsToken))
	if repairEvidence == "" {
		repairEvidence = verifyDetail
	}
	if err := os.WriteFile(filepath.Join(managed.OutputDir, "fanout-repair.txt"), []byte(repairEvidence+"\n"), 0o600); err != nil {
		return err
	}
	// Reports first: a failure copying the log must not cost a finished run its
	// report.json and report.md.
	if err := WriteArtifacts(managed.OutputDir, *report); err != nil {
		return err
	}
	if err := copyEvidenceFile(logPath, filepath.Join(managed.OutputDir, "fanout.log")); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nRemote evidence ready\n  %s\n  %s\n  %s\n  %s\n  %s\n", filepath.Join(managed.OutputDir, "report.json"), filepath.Join(managed.OutputDir, "report.md"), filepath.Join(managed.OutputDir, "samples.jsonl"), filepath.Join(managed.OutputDir, "fanout-repair.txt"), filepath.Join(managed.OutputDir, "fanout.log"))
	if !report.Summary.Passed {
		return errors.New("quality gates failed")
	}
	return nil
}

func managedRunnerConfig(managed ManagedConfig, httpAddr, dataDir string) Config {
	cfg := Config{
		Host: managed.ProvisionedHost, HostDescription: managed.HostDescription,
		SSHPort: managed.ProvisionedSSHPort, SSHIdentity: managed.ProvisionedSSHIdentity,
		Preset: managed.Preset, Repeat: managed.Repeat, BatchSize: managed.BatchSize,
		Endpoint: httpAddr, MetricsURL: "http://" + httpAddr + "/metrics", QueryURL: "http://" + httpAddr,
		Insecure: true, Telemetrygen: managed.TelemetrygenPath, OutputDir: managed.OutputDir,
		FanoutVersion: managed.FanoutVersion, FanoutArtifact: managed.FanoutArtifact, DataDir: dataDir,
		SampleInterval: managed.SampleInterval, GeneratorTimeout: 30 * time.Second, Cooldown: managed.Cooldown,
		QueryRate: managed.QueryRate, SameHost: managed.GeneratorSSH == "", TelemetrygenVersion: managed.TelemetrygenVersion,
		ProcessOverride: managed.ProcessOverride, WorkerOverride: managed.WorkerOverride,
		RateOverride: managed.RateOverride, DurationOverride: managed.DurationOverride,
	}
	cfg.GeneratorSSH = managed.GeneratorSSH
	cfg.GeneratorSSHPort = managed.GeneratorSSHPort
	cfg.GeneratorSSHIdentity = managed.GeneratorSSHIdentity
	cfg.GeneratorSSHKnownHosts = filepath.Join(filepath.Dir(dataDir), "generator_known_hosts")
	cfg.GeneratorHostDescription = managed.GeneratorHostDescription
	if managed.GeneratorSSH != "" {
		cfg.GeneratorHost = "root@<generator-host>"
	}
	return cfg
}

func completeSetup(ctx context.Context, baseURL, setupToken string) (string, string, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 10 * time.Second, Jar: jar}
	payload, _ := json.Marshal(map[string]string{"email": "benchmark@localhost", "display_name": "Fanout Benchmark", "setup_token": setupToken})
	deadline := time.Now().Add(20 * time.Second)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/auth/setup", bytes.NewReader(payload))
		if err != nil {
			return "", "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Fanout-Request", "1")
		resp, err := client.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if readErr != nil {
				return "", "", readErr
			}
			if resp.StatusCode == http.StatusOK {
				var decoded struct {
					IngestToken string `json:"ingest_token"`
				}
				if err := json.Unmarshal(body, &decoded); err != nil {
					return "", "", err
				}
				if decoded.IngestToken == "" {
					return "", "", errors.New("Fanout setup response missing ingest_token")
				}
				parsed, _ := url.Parse(baseURL)
				cookies := jar.Cookies(parsed)
				pairs := make([]string, 0, len(cookies))
				for _, cookie := range cookies {
					pairs = append(pairs, cookie.Name+"="+cookie.Value)
				}
				if len(pairs) == 0 {
					return "", "", errors.New("Fanout setup returned no session cookie")
				}
				return decoded.IngestToken, strings.Join(pairs, "; "), nil
			}
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return "", "", fmt.Errorf("Fanout setup status %d", resp.StatusCode)
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return "", "", fmt.Errorf("Fanout setup: %w", err)
			}
			return "", "", errors.New("Fanout setup timed out")
		}
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func managedListenAddress(ip string) (string, error) {
	if ip == "" {
		ip = "127.0.0.1"
	}
	if parsed := net.ParseIP(ip); parsed == nil || parsed.IsUnspecified() {
		return "", fmt.Errorf("--listen requires a concrete IP address, got %q", ip)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		return "", err
	}
	defer listener.Close()
	return listener.Addr().String(), nil
}
func randomHex(bytesCount int) (string, error) {
	raw := make([]byte, bytesCount)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
func commandOutput(path string, args ...string) string {
	output, _ := exec.Command(path, args...).CombinedOutput()
	return string(output)
}

func copyEvidenceFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	// Truncate rather than refuse: re-running into the same output directory is
	// routine, and O_EXCL would fail here after the whole benchmark has run.
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, io.LimitReader(in, 64<<20))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// Fanout rejects unknown FANOUT_ names. Isolate the managed product from the
// caller's Fanout and benchmark settings, including any AI provider selection.
// Reuse this exact environment for offline repair verification.
func fanoutEnvironment(parent []string, dataDir, addr, authSecret, metricsToken string) []string {
	env := make([]string, 0, len(parent)+7)
	for _, entry := range parent {
		if !strings.HasPrefix(entry, "FANOUT_") {
			env = append(env, entry)
		}
	}
	return append(env,
		"FANOUT_ADDR="+addr,
		"FANOUT_DATA_DIR="+dataDir,
		"FANOUT_AUTH_CODE_SECRET="+authSecret,
		"FANOUT_METRICS_TOKEN="+metricsToken,
		"FANOUT_METRICS_PUBLIC=false",
		"FANOUT_MCP_ENABLED=false",
		"FANOUT_ALERTS_ENABLED=false",
	)
}
