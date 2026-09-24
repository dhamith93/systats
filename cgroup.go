package systats

import (
	"errors"
	"path"
	"strconv"
	"strings"

	"github.com/dhamith93/systats/internal/fileops"
)

// cgroupVersion identifies which cgroup filesystem layout is in use.
// v1 and v2 have completely different paths/file formats, so every
// cgroup-aware code path branches on this first.
type cgroupVersion int

const (
	cgroupNone cgroupVersion = iota
	cgroupV1
	cgroupV2
)

// detectCgroupVersion is the standard way to tell v1 and v2 apart: a v2
// unified hierarchy has a cgroup.controllers file directly at the mount
// root; v1 doesn't. This correctly classifies systemd's "hybrid" mode as
// v1 too, since the real memory/cpu controllers still live on the v1
// side there even though a bookkeeping-only v2 hierarchy also exists.
func detectCgroupVersion(cgroupRootPath string) cgroupVersion {
	if fileops.IsFile(path.Join(cgroupRootPath, "cgroup.controllers")) {
		return cgroupV2
	}
	if fileops.IsFile(path.Join(cgroupRootPath, "memory", "memory.limit_in_bytes")) {
		return cgroupV1
	}
	return cgroupNone
}

// parseSelfCgroupV2 parses /proc/self/cgroup's v2 format: a single line
// "0::<path>".
func parseSelfCgroupV2(content string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(content), ":", 3)
	if len(parts) != 3 || parts[0] != "0" {
		return "", errors.New("unexpected v2 /proc/self/cgroup format")
	}
	return parts[2], nil
}

// parseSelfCgroupV1 finds the line for a specific controller (e.g.
// "memory", "cpu", "cpuacct") in /proc/self/cgroup's v1 format
// ("hierarchy-ID:controller-list:path", one line per hierarchy) and
// returns its cgroup path plus the literal controller-list string, which
// conventionally doubles as the per-hierarchy directory name under the
// cgroup root (e.g. "cpu,cpuacct" when co-mounted, "cpu" when mounted
// separately - some distros do either). Controllers are matched as exact
// elements of the comma-split list, not by substring: "cpuset" contains
// "cpu" but is a different controller entirely.
func parseSelfCgroupV1(content, controller string) (cgPath string, controllerDir string, err error) {
	for _, line := range strings.Split(content, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 {
			continue
		}
		for _, c := range strings.Split(parts[1], ",") {
			if c == controller {
				return parts[2], parts[1], nil
			}
		}
	}
	return "", "", errors.New("controller " + controller + " not found in /proc/self/cgroup")
}

// resolveCgroupControllerPath returns the absolute directory holding a
// given controller's files for the calling process's own cgroup.
// controller is ignored for v2, which has a single unified hierarchy.
func resolveCgroupControllerPath(systats *SyStats, version cgroupVersion, controller string) (dir string, ok bool) {
	selfCgroup, err := fileops.ReadFileWithError(systats.SelfCgroupPath)
	if err != nil {
		return "", false
	}

	switch version {
	case cgroupV2:
		cgPath, err := parseSelfCgroupV2(selfCgroup)
		if err != nil {
			return "", false
		}
		return path.Join(systats.CgroupRootPath, cgPath), true
	case cgroupV1:
		cgPath, controllerDir, err := parseSelfCgroupV1(selfCgroup, controller)
		if err != nil {
			return "", false
		}
		return path.Join(systats.CgroupRootPath, controllerDir, cgPath), true
	default:
		return "", false
	}
}

// defaultV1ControllerDirs is where each v1 controller conventionally lives
// under the cgroup root, used when /proc/self/cgroup can't tell us. cpu and
// cpuacct are co-mounted on essentially every distro that still runs v1.
var defaultV1ControllerDirs = map[string]string{
	"memory":  "memory",
	"cpu":     "cpu,cpuacct",
	"cpuacct": "cpu,cpuacct",
	"pids":    "pids",
	"blkio":   "blkio",
	"freezer": "freezer",
}

// v1ControllerDirNames maps each v1 controller to the directory holding its
// hierarchy under the cgroup root. The directory name is the literal
// controller list from /proc/self/cgroup ("cpu,cpuacct" when co-mounted),
// the same convention resolveCgroupControllerPath relies on. Controllers
// missing from content fall back to defaultV1ControllerDirs.
func v1ControllerDirNames(content string) map[string]string {
	names := make(map[string]string, len(defaultV1ControllerDirs))
	for c, dir := range defaultV1ControllerDirs {
		names[c] = dir
	}
	for _, line := range strings.Split(content, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 || parts[1] == "" {
			continue
		}
		for _, c := range strings.Split(parts[1], ",") {
			if _, known := names[c]; known {
				names[c] = parts[1]
			}
		}
	}
	return names
}

// cgroupDirs locates each controller's directory for one cgroup. Under v2
// they're all the same directory; under v1 each controller has its own
// hierarchy, so the same cgroup lives at a different path per controller.
type cgroupDirs struct {
	version cgroupVersion
	memory  string
	cpu     string
	cpuacct string
	pids    string
	blkio   string
	freezer string
}

// cgroupDirsFor resolves relPath (a cgroup path relative to the hierarchy
// root, as /proc/<pid>/cgroup reports it) to per-controller directories.
// v1Names is only consulted for v1.
func cgroupDirsFor(version cgroupVersion, root, relPath string, v1Names map[string]string) cgroupDirs {
	if version == cgroupV2 {
		dir := path.Join(root, relPath)
		return cgroupDirs{version: version, memory: dir, cpu: dir, cpuacct: dir, pids: dir, blkio: dir, freezer: dir}
	}
	at := func(controller string) string {
		return path.Join(root, v1Names[controller], relPath)
	}
	return cgroupDirs{
		version: version,
		memory:  at("memory"),
		cpu:     at("cpu"),
		cpuacct: at("cpuacct"),
		pids:    at("pids"),
		blkio:   at("blkio"),
		freezer: at("freezer"),
	}
}

// parseKeyValueStat parses the "key value" line format shared by
// memory.stat, cpu.stat, cpuacct.stat, memory.events and friends. Lines
// that aren't exactly two fields, or whose value isn't an unsigned
// integer, are skipped - the kernel adds keys to these files over time.
func parseKeyValueStat(content string) map[string]uint64 {
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
	return stats
}

// readKeyValueStat reads and parses a key/value stat file. A missing or
// unreadable file yields an empty map, so callers can index it freely.
func readKeyValueStat(file string) map[string]uint64 {
	content, err := fileops.ReadFileWithError(file)
	if err != nil {
		return map[string]uint64{}
	}
	return parseKeyValueStat(content)
}
