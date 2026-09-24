package systats

import (
	"errors"
	"path"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"github.com/dhamith93/systats/internal/strops"
	"github.com/dhamith93/systats/internal/unitconv"
)

// Memory holds information on system memory usage.
//
// The size fields are float64 in the requested Unit. They're not integers
// because the larger units can't be represented usefully that way - 4 GiB
// of RAM truncates to "3 GB", losing a fifth of the value.
type Memory struct {
	PercentageUsed float64 `json:"percentageUsed"`
	// Available and Free are the same value when Limited: a cgroup has
	// no "raw free pages" concept distinct from "room left before the
	// limit" the way host /proc/meminfo separately tracks reclaimable
	// buffers/cache.
	Available float64 `json:"available"`
	Free      float64 `json:"free"`
	Used      float64 `json:"used"`
	Time      int64   `json:"time"`
	Total     float64 `json:"total"`
	Unit      Unit    `json:"unit"`
	// Limited is true when SyStats.ContainerAware found a real cgroup
	// memory limit, in which case Total/Used/Available/Free/
	// PercentageUsed reflect that limit instead of host-wide memory.
	Limited bool `json:"limited"`
}

// memoryKiB holds the figures in their native /proc/meminfo unit (KiB)
// before conversion. Keeping them integral until the final conversion
// means cgroup overrides and the percentage are computed on exact
// values, with a single rounding step at the end.
type memoryKiB struct {
	total          uint64
	free           uint64
	available      uint64
	used           uint64
	percentageUsed float64
	limited        bool
}

func getMemory(systats *SyStats, unit Unit) (Memory, error) {
	output := Memory{Unit: unit}

	// Resolved first so an unsupported unit fails before any file I/O.
	convert, err := kibConverter(unit)
	if err != nil {
		return output, err
	}

	meminfoStr, err := fileops.ReadFileWithError(systats.MeminfoPath)
	if err != nil {
		return output, err
	}

	m := parseMeminfo(meminfoStr)
	applyCgroupMemory(&m, systats)

	output.Total = convert(m.total)
	output.Free = convert(m.free)
	output.Available = convert(m.available)
	output.Used = convert(m.used)
	output.PercentageUsed = m.percentageUsed
	output.Limited = m.limited
	output.Time = time.Now().Unix()

	return output, nil
}

// parseMeminfo reads the host-wide figures from /proc/meminfo content.
func parseMeminfo(content string) memoryKiB {
	m := memoryKiB{}
	var buffers, cached uint64

	for _, line := range strings.Split(content, "\n") {
		lineArr := strings.Fields(line)
		if len(lineArr) == 0 {
			continue
		}
		switch lineArr[0] {
		case "MemTotal:":
			m.total = strops.ToUint64(lineArr[1])
		case "MemFree:":
			m.free = strops.ToUint64(lineArr[1])
		case "MemAvailable:":
			m.available = strops.ToUint64(lineArr[1])
		case "Buffers:":
			buffers = strops.ToUint64(lineArr[1])
		case "Cached:":
			cached = strops.ToUint64(lineArr[1])
		}
	}

	if m.total > 0 {
		m.used = m.total - (m.free + buffers + cached)
		m.percentageUsed = float64(m.used) / float64(m.total) * 100
	}

	return m
}

// kibConverter returns the KiB-to-unit conversion for unit, or an error
// for an unrecognized one. All four exported unit constants are
// supported; the conversions are binary (see internal/unitconv).
func kibConverter(unit Unit) (func(uint64) float64, error) {
	switch unit {
	case Byte:
		return unitconv.KibToBytes, nil
	case Kilobyte:
		return unitconv.KibToKB, nil
	case Megabyte:
		return unitconv.KibToMB, nil
	case Gigabyte:
		return unitconv.KibToGB, nil
	default:
		return nil, errors.New(string(unit) + " is not supported")
	}
}

// applyCgroupMemory overrides m with cgroup-relative values when
// ContainerAware is set and a real memory limit is found. Any failure
// along the way (no cgroup, unreadable files, no limit set) leaves m
// exactly as the host-wide parse left it - this never changes
// GetMemory's error behavior or return shape.
func applyCgroupMemory(m *memoryKiB, systats *SyStats) {
	if !systats.ContainerAware {
		return
	}

	version := detectCgroupVersion(systats.CgroupRootPath)
	dir, ok := resolveCgroupControllerPath(systats, version, "memory")
	if !ok {
		return
	}

	limitBytes, limited, usedBytes, err := readCgroupMemoryBytes(version, dir)
	if err != nil || !limited {
		return
	}

	// Cgroup files report raw bytes; the rest of getMemory works in KiB
	// (matching /proc/meminfo's native unit), so convert here to flow
	// through the same unit conversion as the host-wide figures.
	totalKiB := limitBytes / 1024
	usedKiB := usedBytes / 1024
	availableKiB := uint64(0)
	if totalKiB > usedKiB {
		availableKiB = totalKiB - usedKiB
	}

	m.total = totalKiB
	m.used = usedKiB
	m.available = availableKiB
	m.free = availableKiB
	if totalKiB > 0 {
		m.percentageUsed = float64(usedKiB) / float64(totalKiB) * 100
	}
	m.limited = true
}

// readCgroupMemoryBytes reads dir's memory limit and its working-set
// usage (raw usage minus reclaimable inactive file cache). limited is false
// when no limit is configured, in which case limitBytes is 0 but usedBytes
// is still valid - per-container stats want usage either way.
func readCgroupMemoryBytes(version cgroupVersion, dir string) (limitBytes uint64, limited bool, usedBytes uint64, err error) {
	limitBytes, limited, err = readCgroupMemoryLimit(version, dir)
	if err != nil {
		return 0, false, 0, err
	}

	usageBytes, err := readCgroupMemoryUsage(version, dir)
	if err != nil {
		return 0, false, 0, err
	}
	inactiveFileBytes := readCgroupMemoryInactiveFileBytes(version, dir)

	usedBytes = usageBytes
	if inactiveFileBytes < usedBytes {
		usedBytes -= inactiveFileBytes
	}
	return limitBytes, limited, usedBytes, nil
}

// readCgroupMemoryLimit reads the memory limit for dir. limited is false
// when no real limit is configured (v2's literal "max", or v1's
// well-known ~unlimited sentinel, compared with margin since its exact
// low bits depend on page size).
func readCgroupMemoryLimit(version cgroupVersion, dir string) (limitBytes uint64, limited bool, err error) {
	switch version {
	case cgroupV2:
		content, err := fileops.ReadFileWithError(path.Join(dir, "memory.max"))
		if err != nil {
			return 0, false, err
		}
		content = strings.TrimSpace(content)
		if content == "max" {
			return 0, false, nil
		}
		return strops.ToUint64(content), true, nil
	case cgroupV1:
		content, err := fileops.ReadFileWithError(path.Join(dir, "memory.limit_in_bytes"))
		if err != nil {
			return 0, false, err
		}
		value := strops.ToUint64(strings.TrimSpace(content))
		if value > 1<<62 {
			return 0, false, nil
		}
		return value, true, nil
	default:
		return 0, false, errors.New("unknown cgroup version")
	}
}

func readCgroupMemoryUsage(version cgroupVersion, dir string) (uint64, error) {
	filename := "memory.usage_in_bytes"
	if version == cgroupV2 {
		filename = "memory.current"
	}
	content, err := fileops.ReadFileWithError(path.Join(dir, filename))
	if err != nil {
		return 0, err
	}
	return strops.ToUint64(strings.TrimSpace(content)), nil
}

// readCgroupMemoryInactiveFileBytes returns the reclaimable file-cache
// portion of usage, so it can be subtracted out - the same adjustment
// docker stats/cAdvisor make, since raw usage alone makes a container
// that's merely read a lot of files from disk look like it's about to
// OOM. Best-effort: any failure or missing key yields 0, not an error -
// this is a refinement on top of an already-valid usage/limit reading,
// not a required value.
func readCgroupMemoryInactiveFileBytes(version cgroupVersion, dir string) uint64 {
	content, err := fileops.ReadFileWithError(path.Join(dir, "memory.stat"))
	if err != nil {
		return 0
	}

	stats := parseKeyValueStat(content)

	if version == cgroupV1 {
		if v, ok := stats["total_inactive_file"]; ok {
			return v
		}
	}
	return stats["inactive_file"]
}
