package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedListenDerivesAllEndpointsAndPlacement(t *testing.T) {
	for _, ip := range []string{"", "127.0.0.1"} {
		addr, err := managedListenAddress(ip)
		if err != nil {
			t.Fatal(err)
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host != "127.0.0.1" || port == "0" || port == "" {
			t.Fatalf("address=%q, err=%v", addr, err)
		}
	}
	for _, ip := range []string{"hostname", "10.77.0.2:7520", "0.0.0.0", "::"} {
		if _, err := managedListenAddress(ip); err == nil {
			t.Errorf("accepted invalid listen IP %q", ip)
		}
	}
	// These addresses need not belong to the test machine: endpoint derivation
	// is independent of port reservation and works for private IPv4 and IPv6.
	for _, addr := range []string{"10.77.0.2:12345", "[fd77::2]:12345"} {
		for _, split := range []bool{false, true} {
			managed := ManagedConfig{TelemetrygenPath: "/generator/telemetrygen", HostDescription: "Fanout ccx33", GeneratorHostDescription: "Generator ccx43"}
			if split {
				managed.GeneratorSSH = "root@10.77.0.3"
				managed.GeneratorSSHPort = 2222
				managed.GeneratorSSHIdentity = "/stage/generator_key"
			}
			cfg := managedRunnerConfig(managed, addr, "/stage/data")
			if cfg.Endpoint != addr || cfg.MetricsURL != "http://"+addr+"/metrics" || cfg.QueryURL != "http://"+addr {
				t.Fatalf("endpoints=%+v", cfg)
			}
			if cfg.SameHost == split || cfg.GeneratorSSH != managed.GeneratorSSH || cfg.GeneratorSSHPort != managed.GeneratorSSHPort || cfg.GeneratorSSHIdentity != managed.GeneratorSSHIdentity || cfg.GeneratorSSHKnownHosts != filepath.Join("/stage", "generator_known_hosts") {
				t.Fatalf("generator placement=%+v", cfg)
			}
			if cfg.GeneratorHostDescription != managed.GeneratorHostDescription || cfg.HostDescription != managed.HostDescription {
				t.Fatal("managed runner lost host descriptions")
			}
		}
	}
}

func TestFanoutEnvironmentUsesCurrentSchema(t *testing.T) {
	// Schema env tags captured from internal/config/config.go at 870c90db.
	schema, err := os.ReadFile("testdata/fanout-v2026.10.1-alpha-env.txt")
	if err != nil {
		t.Fatal(err)
	}
	knownNames := map[string]bool{}
	for _, name := range strings.Fields(string(schema)) {
		knownNames[name] = true
	}
	want := map[string]string{
		"FANOUT_ADDR":             "127.0.0.1:12345",
		"FANOUT_DATA_DIR":         "/tmp/data",
		"FANOUT_AUTH_CODE_SECRET": "auth-secret",
		"FANOUT_METRICS_TOKEN":    "metrics-secret",
		"FANOUT_METRICS_PUBLIC":   "false",
		"FANOUT_MCP_ENABLED":      "false",
		"FANOUT_ALERTS_ENABLED":   "false",
	}
	parent := []string{"PATH=/bin", "FANOUT_HTTP_ADDR=:7520", "FANOUT_AI_PROVIDER=openai", "FANOUT_BENCH_QUERY_COOKIE=secret", "FANOUT_ADDR=:1", "FANOUT_DATA_DIR=old", "FANOUT_AUTH_CODE_SECRET_FILE=/secret", "FANOUT_METRICS_TOKEN_FILE=/secret"}
	env := fanoutEnvironment(parent, "/tmp/data", "127.0.0.1:12345", "auth-secret", "metrics-secret")
	seen := map[string]bool{}
	for _, entry := range env {
		name, value, _ := strings.Cut(entry, "=")
		if seen[name] {
			t.Fatalf("duplicate setting %s", name)
		}
		seen[name] = true
		if !strings.HasPrefix(name, "FANOUT_") {
			continue
		}
		if !knownNames[name] {
			t.Errorf("unknown Fanout schema setting %s", name)
		}
		expected, known := want[name]
		if !known {
			t.Errorf("unknown or uncontrolled Fanout setting %s", name)
		} else if value != expected {
			t.Errorf("%s=%q, want %q", name, value, expected)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("missing %s", name)
		}
	}
	if !seen["PATH"] {
		t.Error("lost non-Fanout environment")
	}
}

func TestSetupRequiresIngestToken(t *testing.T) {
	for _, response := range []string{`{"status":"authenticated"}`, `{"ingest_token":""}`} {
		t.Run(response, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/api/auth/setup" || r.Method != http.MethodPost || r.Header.Get("Fanout-Request") != "1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["display_name"] != "Fanout Benchmark" || body["email"] != "benchmark@localhost" || body["setup_token"] != "setup-secret" {
					t.Errorf("wrong setup shape: %v", body)
				}
				http.SetCookie(w, &http.Cookie{Name: "fanout_session", Value: "session-secret", Path: "/", HttpOnly: true})
				fmt.Fprint(w, response)
			}))
			defer server.Close()
			token, cookie, err := completeSetup(context.Background(), server.URL, "setup-secret")
			if err == nil || !strings.Contains(err.Error(), "missing ingest_token") {
				t.Fatalf("error=%v, want missing ingest_token", err)
			}
			if token != "" || cookie != "" || requests != 1 {
				t.Fatalf("token=%q cookie=%q requests=%d", token, cookie, requests)
			}
		})
	}
}

func TestMetricsPathAndBearerAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer metrics-secret" {
			http.Error(w, "private", http.StatusUnauthorized)
			return
		}
		fmt.Fprintln(w, `fanout_ingest_rows_total{signal="spans"} 42`)
	}))
	defer server.Close()
	runner := Runner{Config: Config{MetricsURL: server.URL + "/metrics", MetricsToken: "metrics-secret"}, client: server.Client()}
	snap, err := runner.scrape(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acceptedCounts(snap).Spans != 42 {
		t.Fatal("accepted counter not scraped")
	}
	runner.Config.MetricsToken = ""
	if _, err := runner.scrape(context.Background()); err == nil {
		t.Fatal("unauthenticated scrape succeeded")
	}
}
