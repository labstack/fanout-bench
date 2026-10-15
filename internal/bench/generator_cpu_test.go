package bench

import (
	"context"
	"math"
	"slices"
	"testing"
)

func TestGeneratorCPUUsageFromProcStat(t *testing.T) {
	before, err := parseCPUStat("cpu 100 20 30 400 50 6 7 8 90 10\ncpu0 0\ncpu1 0\nintr 123\n")
	if err != nil {
		t.Fatal(err)
	}
	// Guest fields must not contribute to either total; iowait is idle.
	if before.total != 621 || before.idle != 450 || before.logicalCPUs != 2 {
		t.Fatalf("baseline=%+v", before)
	}
	after, err := parseCPUStat("cpu 160 20 50 410 55 8 9 9 150 50\ncpu0 0\ncpu1 0\n")
	if err != nil {
		t.Fatal(err)
	}
	usage, err := generatorCPUUsage(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if usage.LogicalCPUs != 2 || math.Abs(usage.AverageCPUCores-1.7) > 1e-12 {
		t.Fatalf("usage=%+v want 1.7 / 2 cores", usage)
	}
	for _, idle := range []uint64{0, 100} {
		usage, err := generatorCPUUsage(cpuStat{logicalCPUs: 16}, cpuStat{total: 100, idle: idle, logicalCPUs: 16})
		if err != nil || usage.AverageCPUCores != 16*(1-float64(idle)/100) {
			t.Fatalf("idle=%d usage=%+v error=%v", idle, usage, err)
		}
	}
}

func TestGeneratorCPURejectsUnusableSamples(t *testing.T) {
	for _, data := range []string{"", "cpu 1 2 3\ncpu0 0", "cpu 1 bad 3 4\ncpu0 0", "cpu 1 2 3 4", "cpu0 1 2 3 4", "cpu -1 2 3 4\ncpu0 0"} {
		if _, err := parseCPUStat(data); err == nil {
			t.Errorf("accepted malformed stat %q", data)
		}
	}
	before := cpuStat{total: 100, idle: 50, logicalCPUs: 2}
	for _, after := range []cpuStat{
		before,
		{total: 99, idle: 50, logicalCPUs: 2},
		{total: 110, idle: 49, logicalCPUs: 2},
		{total: 110, idle: 61, logicalCPUs: 2},
		{total: 110, idle: 50, logicalCPUs: 3},
		{total: 110, idle: 50},
	} {
		if _, err := generatorCPUUsage(before, after); err == nil {
			t.Errorf("accepted unusable delta: %+v -> %+v", before, after)
		}
	}
}

func TestGeneratorCPUCommandUsesGeneratorSSHSetup(t *testing.T) {
	cfg := Config{GeneratorSSH: "root@10.77.0.3", GeneratorSSHPort: 2222, GeneratorSSHIdentity: "/stage/key", GeneratorSSHKnownHosts: "/stage/known_hosts"}
	load, cancel := generatorCommand(context.Background(), cfg, 0, "--version")
	defer cancel()
	stat := generatorSSHCommand(context.Background(), cfg, "cat /proc/stat")
	if !slices.Equal(load.Args[:len(load.Args)-1], stat.Args[:len(stat.Args)-1]) || stat.Args[len(stat.Args)-1] != "cat /proc/stat" {
		t.Fatalf("CPU command does not reuse generator connection setup: %q", stat.Args)
	}
}
