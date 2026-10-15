package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/labstack/fanout-bench/internal/bench"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "run":
		err = run(ctx, os.Args[2:])
	case "managed":
		err = managed(ctx, os.Args[2:])
	case "doctor":
		err = doctor(ctx, os.Args[2:])
	case "render":
		err = render(os.Args[2:])
	case "version", "--version", "-version":
		fmt.Println(version)
		return
	case "help", "--help", "-h":
		usage()
		return
	default:
		usage()
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `fanout-bench — reproducible OpenTelemetry performance evidence

Usage:
  fanout-bench doctor [flags]  verify telemetrygen, authentication, and metrics
  fanout-bench run [flags]     provision a host and execute a benchmark suite
  fanout-bench render [flags]  regenerate Markdown from report.json

Secrets are accepted only through environment variables and are never persisted.`)
}

func commonFlags(name string, args []string) (bench.Config, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	cfg := bench.Config{}
	var tokenEnv, metricsTokenEnv, queryCookieEnv string
	fs.StringVar(&cfg.Host, "host", "", "SSH target to provision (for example root@example.com); empty runs locally")
	fs.StringVar(&cfg.SSHIdentity, "ssh-identity", "", "SSH private key path")
	fs.StringVar(&cfg.SSHKnownHosts, "ssh-known-hosts", "", "run-scoped SSH known-hosts file")
	fs.StringVar(&cfg.HostDescription, "host-description", "", "host description recorded alongside hardware evidence")
	if name == "run" {
		fs.StringVar(&cfg.GeneratorHost, "generator-host", "", "SSH target to provision as a separate generator host")
		fs.StringVar(&cfg.GeneratorAddress, "generator-address", "", "generator host IP reachable from the Fanout host")
		fs.StringVar(&cfg.FanoutListen, "fanout-listen", "", "Fanout host IP reachable from the generator host")
		fs.StringVar(&cfg.GeneratorHostDescription, "generator-host-description", "", "generator host description recorded in the report")
	}
	fs.IntVar(&cfg.SSHPort, "ssh-port", 22, "SSH port")
	fs.BoolVar(&cfg.KeepRemote, "keep-remote", false, "preserve the isolated remote run directory after downloading artifacts")
	fs.StringVar(&cfg.Preset, "preset", "standard", "suite: quick, standard, or publish")
	fs.StringVar(&cfg.Endpoint, "endpoint", "localhost:7520", "Fanout OTLP/gRPC endpoint")
	fs.StringVar(&cfg.MetricsURL, "metrics-url", "http://localhost:7520/metrics", "Fanout Prometheus metrics URL (required evidence source)")
	fs.BoolVar(&cfg.Insecure, "insecure", false, "use plaintext OTLP/gRPC")
	fs.StringVar(&cfg.Telemetrygen, "telemetrygen", "./bin/telemetrygen", "path to pinned telemetrygen binary")
	fs.StringVar(&cfg.TelemetrygenVersion, "telemetrygen-version", "v0.157.0", "version recorded for the generator binary")
	fs.StringVar(&cfg.OutputDir, "output", "", "artifact directory (run command only)")
	fs.StringVar(&tokenEnv, "token-env", "FANOUT_BENCH_INGEST_TOKEN", "environment variable containing the ingest bearer token")
	fs.StringVar(&metricsTokenEnv, "metrics-token-env", "FANOUT_BENCH_METRICS_TOKEN", "environment variable containing an optional metrics bearer token")
	fs.StringVar(&cfg.FanoutVersion, "fanout-version", "unknown", "Fanout release under test")
	fs.StringVar(&cfg.FanoutCommit, "fanout-commit", "", "Fanout git commit under test")
	fs.StringVar(&cfg.FanoutArtifact, "fanout-artifact", "", "Fanout binary or release artifact identity under test")
	fs.StringVar(&cfg.DataDir, "data-dir", "", "local Fanout data directory for disk-growth evidence")
	fs.DurationVar(&cfg.SampleInterval, "sample-interval", time.Second, "server resource sampling interval")
	fs.DurationVar(&cfg.GeneratorTimeout, "export-timeout", 30*time.Second, "telemetrygen export timeout")
	fs.DurationVar(&cfg.Cooldown, "cooldown", 15*time.Second, "post-load retained-memory observation window")
	fs.IntVar(&cfg.Repeat, "repeat", 1, "repeat every sweep topology")
	fs.BoolVar(&cfg.SameHost, "same-host", false, "declare that Fanout and generators share a host")
	fs.IntVar(&cfg.BatchSize, "batch-size", 1000, "telemetrygen OTLP batch size")
	fs.StringVar(&cfg.QueryURL, "query-url", "", "Fanout HTTP base URL for authenticated read-under-write load")
	fs.Float64Var(&cfg.QueryRate, "query-rate", 0, "aggregate UI operations/s during mixed phases")
	fs.StringVar(&queryCookieEnv, "query-cookie-env", "FANOUT_BENCH_QUERY_COOKIE", "environment variable containing the authenticated session Cookie header")
	fs.IntVar(&cfg.ProcessOverride, "processes", 0, "override sweep with one process count per signal")
	fs.IntVar(&cfg.WorkerOverride, "workers", 0, "override workers per telemetrygen process")
	fs.Float64Var(&cfg.RateOverride, "rate", 0, "override items/s per telemetrygen worker")
	fs.DurationVar(&cfg.DurationOverride, "duration", 0, "override every measurement duration (use for diagnostics)")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if err := bench.ValidateGeneratorHosts(cfg); err != nil {
		return cfg, err
	}
	cfg.Token = os.Getenv(tokenEnv)
	cfg.MetricsToken = os.Getenv(metricsTokenEnv)
	cfg.QueryCookie = os.Getenv(queryCookieEnv)
	return cfg, nil
}

func run(ctx context.Context, args []string) error {
	cfg, err := commonFlags("run", args)
	if err != nil {
		return err
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = filepath.Join("results", time.Now().UTC().Format("20060102T150405Z"))
	}
	runner := bench.Runner{Config: cfg, Version: version, Out: os.Stdout}
	if cfg.Host != "" {
		return bench.RunRemote(ctx, cfg, version, os.Stdout)
	}
	report, runErr := runner.Run(ctx)
	if report == nil {
		return runErr
	}
	// A partial report is still written: the suite is tens of minutes of load,
	// and the phases that did complete are worth keeping. Such a report is
	// marked failed and says why.
	if err := bench.WriteArtifacts(cfg.OutputDir, *report); err != nil {
		if runErr != nil {
			return runErr
		}
		return fmt.Errorf("write artifacts: %w", err)
	}
	if runErr != nil {
		fmt.Printf("\nPartial artifacts\n  %s\n  %s\n", filepath.Join(cfg.OutputDir, "report.json"), filepath.Join(cfg.OutputDir, "report.md"))
		return runErr
	}
	fmt.Printf("\nArtifacts\n  %s\n  %s\n  %s\n", filepath.Join(cfg.OutputDir, "report.json"), filepath.Join(cfg.OutputDir, "report.md"), filepath.Join(cfg.OutputDir, "samples.jsonl"))
	if !report.Summary.Passed {
		return fmt.Errorf("quality gates failed")
	}
	return nil
}

func managed(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("managed", flag.ContinueOnError)
	var cfg bench.ManagedConfig
	fs.StringVar(&cfg.FanoutPath, "fanout", "./fanout", "provisioned Fanout binary")
	fs.StringVar(&cfg.TelemetrygenPath, "telemetrygen", "./telemetrygen", "provisioned telemetrygen binary")
	fs.StringVar(&cfg.Listen, "listen", "127.0.0.1", "Fanout listen IP for OTLP, metrics, and queries")
	fs.StringVar(&cfg.GeneratorSSH, "generator-ssh", "", "SSH target for telemetrygen launches")
	fs.IntVar(&cfg.GeneratorSSHPort, "generator-ssh-port", 22, "SSH port for telemetrygen launches")
	fs.StringVar(&cfg.GeneratorSSHIdentity, "generator-ssh-identity", "", "ephemeral SSH private key for the generator host")
	fs.StringVar(&cfg.GeneratorTelemetrygen, "generator-telemetrygen", "", "telemetrygen path on the generator host")
	fs.StringVar(&cfg.OutputDir, "output", "./results", "remote evidence directory")
	fs.StringVar(&cfg.FanoutVersion, "fanout-version", "unknown", "Fanout version")
	fs.StringVar(&cfg.FanoutArtifact, "fanout-artifact", "", "verified release artifact identity")
	fs.StringVar(&cfg.Preset, "preset", "standard", "suite preset")
	fs.IntVar(&cfg.Repeat, "repeat", 1, "sweep repetitions")
	fs.IntVar(&cfg.BatchSize, "batch-size", 1000, "OTLP batch size")
	fs.Float64Var(&cfg.QueryRate, "query-rate", 0, "read-under-write UI operations/s")
	fs.DurationVar(&cfg.SampleInterval, "sample-interval", time.Second, "resource sample interval")
	fs.DurationVar(&cfg.Cooldown, "cooldown", 15*time.Second, "post-load cooldown")
	fs.StringVar(&cfg.HarnessVersion, "harness-version", version, "harness version")
	fs.StringVar(&cfg.TelemetrygenVersion, "telemetrygen-version", "v0.157.0", "telemetrygen version")
	fs.StringVar(&cfg.ProvisionedHost, "provisioned-host", "", "SSH target used to construct the redacted reproduction command")
	fs.StringVar(&cfg.HostDescription, "host-description", "", "host description recorded alongside hardware evidence")
	fs.StringVar(&cfg.GeneratorHostDescription, "generator-host-description", "", "generator host description recorded in the report")
	fs.IntVar(&cfg.ProvisionedSSHPort, "provisioned-ssh-port", 22, "SSH port recorded in remote evidence")
	fs.StringVar(&cfg.ProvisionedSSHIdentity, "provisioned-ssh-identity", "", "SSH identity path recorded in remote evidence")
	fs.IntVar(&cfg.ProcessOverride, "processes", 0, "override process count per signal")
	fs.IntVar(&cfg.WorkerOverride, "workers", 0, "override workers per process")
	fs.Float64Var(&cfg.RateOverride, "rate", 0, "override items per worker per second")
	fs.DurationVar(&cfg.DurationOverride, "duration", 0, "override measurement duration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return bench.RunManaged(ctx, cfg, os.Stdout)
}

func doctor(ctx context.Context, args []string) error {
	cfg, err := commonFlags("doctor", args)
	if err != nil {
		return err
	}
	return bench.Doctor(ctx, cfg, os.Stdout)
}

func render(args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	input := fs.String("input", "report.json", "input JSON report")
	output := fs.String("output", "", "output Markdown path (stdout when empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := bench.LoadReport(*input)
	if err != nil {
		return err
	}
	markdown := bench.Markdown(report)
	if *output == "" {
		fmt.Print(markdown)
		return nil
	}
	return os.WriteFile(*output, []byte(markdown), 0o644)
}
