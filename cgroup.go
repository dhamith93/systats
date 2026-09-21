package systats

import (
	"errors"
	"path"
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
