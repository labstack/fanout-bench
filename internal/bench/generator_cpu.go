package bench

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type cpuStat struct {
	total       uint64
	idle        uint64
	logicalCPUs int
}

func (r *Runner) generatorCPUStat(ctx context.Context) (cpuStat, error) {
	if r.readGeneratorCPU != nil {
		return r.readGeneratorCPU(ctx)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := generatorSSHCommand(ctx, r.Config, "cat /proc/stat")
	output, err := cmd.Output()
	if err != nil {
		return cpuStat{}, err
	}
	return parseCPUStat(string(output))
}

func parseCPUStat(data string) (cpuStat, error) {
	var stat cpuStat
	found := false
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] != "cpu" {
			if suffix, ok := strings.CutPrefix(fields[0], "cpu"); ok {
				if n, err := strconv.Atoi(suffix); err == nil && n >= 0 {
					stat.logicalCPUs++
				}
			}
			continue
		}
		if len(fields) < 5 {
			return cpuStat{}, fmt.Errorf("/proc/stat CPU line has fewer than four counters")
		}
		found = true
		// user, nice, system, idle, iowait, irq, softirq, steal.
		// guest and guest_nice are already included in user and nice.
		for i := 1; i < len(fields) && i <= 8; i++ {
			value, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				return cpuStat{}, fmt.Errorf("/proc/stat CPU counter: %w", err)
			}
			stat.total += value
			if i == 4 || i == 5 {
				stat.idle += value
			}
		}
	}
	if !found || stat.logicalCPUs == 0 {
		return cpuStat{}, fmt.Errorf("/proc/stat is missing aggregate CPU counters or logical CPUs")
	}
	return stat, nil
}

func generatorCPUUsage(before, after cpuStat) (*GeneratorCPU, error) {
	if before.logicalCPUs <= 0 || before.logicalCPUs != after.logicalCPUs {
		return nil, fmt.Errorf("generator logical CPU count missing or changed during trial")
	}
	if after.total <= before.total || after.idle < before.idle {
		return nil, fmt.Errorf("generator CPU counters did not advance or moved backwards")
	}
	total, idle := after.total-before.total, after.idle-before.idle
	if idle > total {
		return nil, fmt.Errorf("generator idle CPU delta exceeds total CPU delta")
	}
	return &GeneratorCPU{
		AverageCPUCores: float64(before.logicalCPUs) * float64(total-idle) / float64(total),
		LogicalCPUs:     before.logicalCPUs,
	}, nil
}
