package main

import "testing"

func TestLocalDefaultsUseSharedFanoutListener(t *testing.T) {
	cfg, err := commonFlags("test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Endpoint != "localhost:7520" || cfg.MetricsURL != "http://localhost:7520/metrics" {
		t.Fatalf("endpoint=%q metrics=%q", cfg.Endpoint, cfg.MetricsURL)
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
