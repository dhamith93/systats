package systats

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dhamith93/systats/internal/fileops"
)

// maxContainerCgroupDepth bounds the cgroup tree walk. The deepest layout
// in practice is Kubernetes under systemd
// (kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod<uid>.slice/cri-containerd-<id>.scope),
// four levels down; the headroom covers nested setups without letting a
// pathological tree turn discovery into a full filesystem crawl.
const maxContainerCgroupDepth = 10

// containerCgroupPattern recognizes one runtime's naming scheme for a
// container's cgroup directory. The first capture group is the container
// ID (or name, for runtimes that have no hex ID).
type containerCgroupPattern struct {
	re      *regexp.Regexp
	runtime string
	// parent, when set, must be the name of the directory directly above -
	// cgroupfs-driver Docker names the cgroup with the bare ID, which is
	// only unambiguous together with its "docker" parent.
	parent string
}

// containerCgroupPatterns covers the systemd and cgroupfs drivers of the
// common runtimes. IDs are anchored to exactly 64 hex characters, which is
// what keeps sibling bookkeeping cgroups such as libpod-conmon-<id>.scope
// from being reported as containers.
var containerCgroupPatterns = []containerCgroupPattern{
	{re: regexp.MustCompile(`^docker-([0-9a-f]{64})\.scope$`), runtime: "docker"},
	{re: regexp.MustCompile(`^([0-9a-f]{64})$`), runtime: "docker", parent: "docker"},
	{re: regexp.MustCompile(`^libpod-([0-9a-f]{64})(?:\.scope)?$`), runtime: "podman"},
	{re: regexp.MustCompile(`^cri-containerd-([0-9a-f]{64})\.scope$`), runtime: "containerd"},
	{re: regexp.MustCompile(`^crio-([0-9a-f]{64})\.scope$`), runtime: "cri-o"},
	{re: regexp.MustCompile(`^lxc\.payload\.(.+)$`), runtime: "lxc"},
	{re: regexp.MustCompile(`^machine-(.+)\.scope$`), runtime: "machined", parent: "machine.slice"},
}

// kubepodsBareIDPattern matches a container under the cgroupfs-driver
// kubelet layout (kubepods/<qos>/pod<uid>/<id>), where the directory name
// is the bare ID and says nothing about which CRI runtime created it.
var kubepodsBareIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// podUIDPattern finds a Kubernetes pod UID in a cgroup path component.
// The systemd driver escapes the UID's dashes as underscores
// (kubepods-burstable-pod1234_5678_....slice), so both are accepted.
var podUIDPattern = regexp.MustCompile(`pod([0-9a-f]{8}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{12})`)

// containerRef is a discovered container: where its cgroup is and what the
// path says about it. Stats are read later, against dirs.
type containerRef struct {
	id      string
	runtime string
	podUID  string
	relPath string
	dirs    cgroupDirs
	pid     int
}

// matchContainerCgroup reports whether the cgroup at relPath is a
// container's, and if so its ID and runtime.
func matchContainerCgroup(relPath string) (id, runtime string, ok bool) {
	name := path.Base(relPath)
	parent := path.Base(path.Dir(relPath))

	for _, p := range containerCgroupPatterns {
		if p.parent != "" && p.parent != parent {
			continue
		}
		if m := p.re.FindStringSubmatch(name); m != nil {
			return unescapeSystemdUnitName(m[1]), p.runtime, true
		}
	}

	if kubepodsBareIDPattern.MatchString(name) && strings.Contains(relPath, "kubepods") {
		return name, "kubernetes", true
	}
	return "", "", false
}

// podUIDFromCgroupPath returns the Kubernetes pod UID embedded in relPath,
// normalized to the dashed form the API server uses, or "" outside
// kubepods.
func podUIDFromCgroupPath(relPath string) string {
	m := podUIDPattern.FindStringSubmatch(relPath)
	if m == nil {
		return ""
	}
	return strings.ReplaceAll(m[1], "_", "-")
}

// unescapeSystemdUnitName reverses the one escape that shows up in
// practice in machine names: systemd writes "-" inside a unit name
// component as "\x2d".
func unescapeSystemdUnitName(s string) string {
	return strings.ReplaceAll(s, `\x2d`, "-")
}

// discoverContainers walks the cgroup hierarchy for container cgroups. It
// never descends into a match - cgroup accounting is hierarchical, so the
// matched directory's figures already include anything nested below it.
// Cgroups with no processes anywhere in their subtree are skipped: they
// are containers that have stopped but whose cgroup hasn't been removed
// yet.
func discoverContainers(systats *SyStats, version cgroupVersion) []containerRef {
	root := systats.CgroupRootPath
	v1Names := map[string]string{}
	walkRoot := root
	if version == cgroupV1 {
		v1Names = v1ControllerDirNames(fileops.ReadFile(systats.SelfCgroupPath))
		walkRoot = path.Join(root, v1Names["memory"])
	}

	refs := []containerRef{}
	_ = filepath.WalkDir(walkRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory hides only its own subtree; keep
			// walking the rest.
			if d != nil && d.IsDir() && p != walkRoot {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() || p == walkRoot {
			return nil
		}

		rel, relErr := filepath.Rel(walkRoot, p)
		if relErr != nil {
			return filepath.SkipDir
		}
		rel = "/" + filepath.ToSlash(rel)
		if strings.Count(rel, "/") > maxContainerCgroupDepth {
			return filepath.SkipDir
		}

		id, runtime, ok := matchContainerCgroup(rel)
		if !ok {
			return nil
		}

		// A v1 container's processes are listed in each controller's copy of
		// its cgroup; the memory hierarchy (the one being walked) is as
		// good as any.
		pid, ok := firstPidInSubtree(p)
		if ok {
			refs = append(refs, containerRef{
				id:      id,
				runtime: runtime,
				podUID:  podUIDFromCgroupPath(rel),
				relPath: rel,
				dirs:    cgroupDirsFor(version, root, rel, v1Names),
				pid:     pid,
			})
		}
		return filepath.SkipDir
	})
	return refs
}

// firstPidInSubtree returns a pid from dir's cgroup.procs, or from the
// first descendant that has one. Under v2 a container's processes may sit
// in a child cgroup (Podman's "container" sub-cgroup, for one) because the
// no-internal-process rule keeps them out of a cgroup that has children.
func firstPidInSubtree(dir string) (int, bool) {
	pid := 0
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || pid != 0 {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		if first, ok := firstPid(path.Join(p, "cgroup.procs")); ok {
			pid = first
			return filepath.SkipDir
		}
		return nil
	})
	return pid, pid != 0
}

func firstPid(procsFile string) (int, bool) {
	content, err := os.ReadFile(procsFile)
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(content), "\n") {
		if pid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}
