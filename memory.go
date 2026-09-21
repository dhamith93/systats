package systats

import (
	"errors"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"github.com/dhamith93/systats/internal/strops"
	"github.com/dhamith93/systats/internal/unitconv"
)

// Memory holds information on system memory usage
type Memory struct {
	PercentageUsed float64 `json:"percentageUsed"`
	// Available and Free are the same value when Limited: a cgroup has
	// no "raw free pages" concept distinct from "room left before the
	// limit" the way host /proc/meminfo separately tracks reclaimable
	// buffers/cache.
	Available uint64 `json:"available"`
	Free      uint64 `json:"free"`
	Used      uint64 `json:"used"`
	Time      int64  `json:"time"`
	Total     uint64 `json:"total"`
	Unit      string `json:"unit"`
	// Limited is true when SyStats.ContainerAware found a real cgroup
	// memory limit, in which case Total/Used/Available/Free/
	// PercentageUsed reflect that limit instead of host-wide memory.
	Limited bool `json:"limited"`
}

func getMemory(systats *SyStats, unit string) (Memory, error) {
	output := Memory{}
	output.Unit = unit

	meminfoStr, err := fileops.ReadFileWithError(systats.MeminfoPath)
	if err != nil {
		return output, err
	}

	meminfoSplit := strings.Split(meminfoStr, "\n")
	var buffers, cached uint64

	for _, line := range meminfoSplit {
		lineArr := strings.Fields(line)
		if len(lineArr) == 0 {
			continue
		}
		if lineArr[0] == "MemTotal:" {
			output.Total = strops.ToUint64(lineArr[1])
		}
		if lineArr[0] == "MemFree:" {
			output.Free = strops.ToUint64(lineArr[1])
		}
		if lineArr[0] == "MemAvailable:" {
			output.Available = strops.ToUint64(lineArr[1])
		}
		if lineArr[0] == "Buffers:" {
			buffers = strops.ToUint64(lineArr[1])
		}
		if lineArr[0] == "Cached:" {
			cached = strops.ToUint64(lineArr[1])
		}
	}

	if output.Total > 0 {
		output.Used = output.Total - (output.Free + buffers + cached)
		percentage := float64(output.Used) / float64(output.Total) * 100
		output.PercentageUsed = percentage
	}

	output.Time = time.Now().Unix()

	applyCgroupMemory(&output, systats)

	if unit == Kilobyte {
		output.Available = unitconv.KibToKB(output.Available)
		output.Total = unitconv.KibToKB(output.Total)
		output.Used = unitconv.KibToKB(output.Used)
		output.Free = unitconv.KibToKB(output.Free)
	} else if unit == Megabyte {
		output.Available = unitconv.KibToMB(output.Available)
		output.Total = unitconv.KibToMB(output.Total)
		output.Used = unitconv.KibToMB(output.Used)
		output.Free = unitconv.KibToMB(output.Free)
	} else {
		return output, errors.New(unit + " is not supported")
	}

	return output, nil
}

// applyCgroupMemory overrides output's fields with cgroup-relative values
// when ContainerAware is set and a real memory limit is found. Any
// failure along the way (no cgroup, unreadable files, no limit set)
// leaves output exactly as the host-wide computation above already left
// it - this never changes GetMemory's error behavior or return shape.
func applyCgroupMemory(output *Memory, systats *SyStats) {
	if !systats.ContainerAware {
		return
	}

	version := detectCgroupVersion(systats.CgroupRootPath)
	dir, ok := resolveCgroupControllerPath(systats, version, "memory")
	if !ok {
		return
	}

	limitBytes, limited, err := readCgroupMemoryLimit(version, dir)
	if err != nil || !limited {
		return
	}

	usageBytes, err := readCgroupMemoryUsage(version, dir)
	if err != nil {
		return
	}
	inactiveFileBytes := readCgroupMemoryInactiveFileBytes(version, dir)

	usedBytes := usageBytes
	if inactiveFileBytes < usedBytes {
		usedBytes -= inactiveFileBytes
	}

	// Cgroup files report raw bytes; the rest of getMemory works in KiB
	// (matching /proc/meminfo's native unit), so convert here to flow
	// through the existing Kilobyte/Megabyte switch below unchanged.
	totalKiB := limitBytes / 1024
	usedKiB := usedBytes / 1024
	availableKiB := uint64(0)
	if totalKiB > usedKiB {
		availableKiB = totalKiB - usedKiB
	}

	output.Total = totalKiB
	output.Used = usedKiB
	output.Available = availableKiB
	output.Free = availableKiB
	if totalKiB > 0 {
		output.PercentageUsed = float64(usedKiB) / float64(totalKiB) * 100
	}
	output.Limited = true
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

	stats := map[string]uint64{}
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		stats[fields[0]] = value
	}

	if version == cgroupV1 {
		if v, ok := stats["total_inactive_file"]; ok {
			return v
		}
	}
	return stats["inactive_file"]
}
