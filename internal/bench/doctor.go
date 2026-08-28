package bench

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func Doctor(ctx context.Context, cfg Config, out io.Writer) error {
	type check struct {
		name string
		run  func() error
	}
	runner := &Runner{Config: cfg, Out: out, client: nil}
	checks := []check{
		{"ingest token", func() error {
			if cfg.Token == "" {
				return fmt.Errorf("missing")
			}
			return nil
		}},
		{"telemetrygen binary", func() error {
			info, err := os.Stat(cfg.Telemetrygen)
			if err != nil {
				return err
			}
			if info.IsDir() {
				return fmt.Errorf("is a directory")
			}
			return nil
		}},
		{"telemetrygen command", func() error {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			output, err := exec.CommandContext(cctx, cfg.Telemetrygen, "--help").CombinedOutput()
			if err != nil {
				return err
			}
			if !strings.Contains(string(output), "telemetrygen") {
				return fmt.Errorf("unexpected help output")
			}
			return nil
		}},
		{"Fanout metrics", func() error {
			if runner.client == nil {
				runner.client = defaultHTTPClient()
			}
			snap, err := runner.scrape(ctx)
			if err != nil {
				return err
			}
			if len(snap.samples) == 0 {
				return fmt.Errorf("empty response")
			}
			return nil
		}},
		{"accepted-row counter", func() error {
			if runner.client == nil {
				runner.client = defaultHTTPClient()
			}
			snap, err := runner.scrape(ctx)
			if err != nil {
				return err
			}
			found := false
			for _, s := range snap.samples {
				if s.name == "fanout_ingest_rows_total" {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("fanout_ingest_rows_total absent")
			}
			return nil
		}},
	}
	fmt.Fprintln(out, "Fanout Bench doctor")
	failed := false
	for _, check := range checks {
		if err := check.run(); err != nil {
			fmt.Fprintf(out, "  ✗ %-24s %v\n", check.name, err)
			failed = true
		} else {
			fmt.Fprintf(out, "  ✓ %s\n", check.name)
		}
	}
	if failed {
		return fmt.Errorf("preflight failed")
	}
	fmt.Fprintln(out, "\nReady. Metrics are authoritative; secrets will not be written to reports.")
	return nil
}

func defaultHTTPClient() *http.Client { return &http.Client{Timeout: 15 * time.Second} }
