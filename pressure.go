package systats

import (
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
)

// PressureMetric is one "some" or "full" line of a PSI file: the
// percentage of time tasks were stalled on a resource, averaged over the
// last 10, 60 and 300 seconds, plus the cumulative stall time.
type PressureMetric struct {
	// Avg10, Avg60 and Avg300 are percentages of wall time (0-100).
	Avg10  float64 `json:"avg10"`
	Avg60  float64 `json:"avg60"`
	Avg300 float64 `json:"avg300"`
	// Total is cumulative stall time in microseconds since boot. Unlike
	// the averages it never resets, so it's the one to diff between polls.
	Total uint64 `json:"total"`
}

// ResourcePressure holds the two stall measures the kernel reports per
// resource.
//
// Some is the share of time at least one task was stalled - the early
// warning. Full is the share of time *every* runnable task was stalled,
// meaning nothing useful happened at all; that's the one that correlates
// with visible slowness.
type ResourcePressure struct {
	Some PressureMetric `json:"some"`
	Full PressureMetric `json:"full"`
	// FullAvailable is false when the kernel reported no "full" line.
	// /proc/pressure/cpu has none on most kernels (full CPU pressure is
	// meaningless system-wide), so check this before reading Full's zeros
	// as "no pressure".
	FullAvailable bool `json:"fullAvailable"`
}

// Pressure holds Pressure Stall Information from /proc/pressure - how
// much time tasks spent waiting on CPU, memory and I/O.
//
// This answers "is this box actually saturated", which load average and
// utilization percentages don't: a machine can sit at 100% CPU and be
// perfectly healthy, or at 40% and be thrashing on memory reclaim.
type Pressure struct {
	CPU    ResourcePressure `json:"cpu"`
	Memory ResourcePressure `json:"memory"`
	IO     ResourcePressure `json:"io"`
	// Available is false when the kernel doesn't expose PSI at all -
	// it needs 4.20+ with CONFIG_PSI=y, and some distros ship it disabled
	// behind a psi=1 boot flag. Not an error: an older kernel is a normal
	// condition, so check this before reading the zeros as real.
	Available bool `json:"available"`
	// Limited is true when the figures came from the calling process's
	// own cgroup (ContainerAware) rather than host-wide.
	Limited bool  `json:"limited"`
	Time    int64 `json:"time"`
}

// psiResource is one resource's PSI file, named differently host-wide
// (/proc/pressure/cpu) and per-cgroup (<cgroup>/cpu.pressure).
type psiResource struct {
	name   string
	target *ResourcePressure
}

func getPressure(systats *SyStats) (Pressure, error) {
	output := Pressure{Time: time.Now().Unix()}

	resources := []psiResource{
		{"cpu", &output.CPU},
		{"memory", &output.Memory},
		{"io", &output.IO},
	}

	dir, cgroupScoped := pressureDir(systats)
	for _, r := range resources {
		content, err := fileops.ReadFileWithError(pressureFilePath(dir, r.name, cgroupScoped))
		if err != nil {
			continue // this resource isn't exposed; Available stays as-is
		}
		*r.target = parsePressure(content)
		output.Available = true
	}
	output.Limited = output.Available && cgroupScoped

	return output, nil
}

// pressureDir picks the directory to read PSI from: the calling process's
// own cgroup when ContainerAware finds one, else the host-wide
// /proc/pressure. Falls back to host-wide silently, matching how
// GetMemory/GetCPU behave when there's no cgroup.
func pressureDir(systats *SyStats) (dir string, cgroupScoped bool) {
	if !systats.ContainerAware {
		return systats.PressurePath, false
	}
	// Only cgroup v2 exposes PSI; v1 has no equivalent.
	if detectCgroupVersion(systats.CgroupRootPath) != cgroupV2 {
		return systats.PressurePath, false
	}
	cgDir, ok := resolveCgroupControllerPath(systats, cgroupV2, "")
	if !ok || !fileops.IsFile(path.Join(cgDir, "cpu.pressure")) {
		return systats.PressurePath, false
	}
	return cgDir, true
}

func pressureFilePath(dir, resource string, cgroupScoped bool) string {
	if cgroupScoped {
		return path.Join(dir, resource+".pressure")
	}
	return path.Join(dir, resource)
}

// parsePressure reads a PSI file body:
//
//	some avg10=0.00 avg60=0.00 avg300=0.00 total=0
//	full avg10=0.00 avg60=0.00 avg300=0.00 total=0
//
// Unknown keys are skipped rather than failing - the kernel has added
// fields here before and may again.
func parsePressure(content string) ResourcePressure {
	out := ResourcePressure{}

	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		var target *PressureMetric
		switch fields[0] {
		case "some":
			target = &out.Some
		case "full":
			target = &out.Full
			out.FullAvailable = true
		default:
			continue
		}

		for _, field := range fields[1:] {
			key, value, found := strings.Cut(field, "=")
			if !found {
				continue
			}
			switch key {
			case "avg10":
				target.Avg10 = parseFloatOrZero(value)
			case "avg60":
				target.Avg60 = parseFloatOrZero(value)
			case "avg300":
				target.Avg300 = parseFloatOrZero(value)
			case "total":
				target.Total, _ = strconv.ParseUint(value, 10, 64)
			}
		}
	}

	return out
}

// parseFloatOrZero is deliberately lenient rather than using
// strops.ToFloat64: a single malformed average shouldn't take out the
// whole reading, and PSI is diagnostic rather than load-bearing.
func parseFloatOrZero(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
