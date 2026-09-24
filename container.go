package systats

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"golang.org/x/sys/unix"
)

// Container is one container running on this host, as seen from the host:
// its identity, and CPU, memory, network, block I/O, mount and pid usage
// read from its cgroup and its processes' view of /proc.
type Container struct {
	// ID is the runtime's full container ID. For LXC and systemd-machined
	// containers, which have no hex ID, it's the container name.
	ID      string `json:"id"`
	ShortID string `json:"shortId"`
	// Name, Image and Labels come from the runtime API and are empty
	// unless MetadataAvailable.
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Labels map[string]string `json:"labels"`
	// State is the runtime's state when MetadataAvailable, otherwise
	// "running", or "paused" when the cgroup is frozen. Stopped containers
	// have no processes and are not listed at all.
	State string `json:"state"`
	// Runtime is inferred from the cgroup layout: "docker", "podman",
	// "containerd", "cri-o", "kubernetes" (a kubelet cgroupfs layout that
	// doesn't name the runtime), "lxc" or "machined".
	Runtime string `json:"runtime"`
	// PodUID is the Kubernetes pod UID, for containers under kubepods.
	PodUID string `json:"podUid"`
	// MetadataAvailable is true when the runtime API on
	// SyStats.ContainerSocketPath answered and knew this container.
	MetadataAvailable bool `json:"metadataAvailable"`
	// CgroupPath is the container's cgroup, relative to the cgroup root
	// (for v1, relative to each controller's hierarchy).
	CgroupPath string `json:"cgroupPath"`
	// Pid is one process in the container, as the host numbers it - the
	// one whose /proc entry network and mount figures were read through.
	Pid int `json:"pid"`

	CPU     ContainerCPU       `json:"cpu"`
	Memory  ContainerMemory    `json:"memory"`
	Network ContainerNetwork   `json:"network"`
	BlockIO []ContainerBlockIO `json:"blockIo"`
	Mounts  []ContainerMount   `json:"mounts"`
	// Layer is the container's own disk usage - its writable layer. Only
	// measured when SyStats.ContainerLayerSize is set.
	Layer ContainerLayer `json:"layer"`
	Pids  ContainerPids  `json:"pids"`
	// Pressure is the container's own PSI. cgroup v1 has none, so check
	// Pressure.Available.
	Pressure Pressure `json:"pressure"`
	Time     int64    `json:"time"`
}

// ContainerCPU is a container's CPU usage over the sampling window, plus
// its cumulative counters.
type ContainerCPU struct {
	// CoresUsed is how many cores' worth of CPU the container used over the
	// sampling window (1.5 == one and a half cores busy). Multiply by 100
	// for the figure docker stats shows as CPU %.
	CoresUsed float64 `json:"coresUsed"`
	// PercentOfHost is CoresUsed as a share of every logical CPU on the
	// host (0-100).
	PercentOfHost float64 `json:"percentOfHost"`
	// PercentOfLimit is CoresUsed as a share of AllocatedCores, 0 when not
	// Limited. Sustained values near 100 mean the container is being
	// throttled - see ThrottledPeriods.
	PercentOfLimit float64 `json:"percentOfLimit"`
	Limited        bool    `json:"limited"`
	// AllocatedCores is the CPU quota in cores (docker --cpus), 0 when not
	// Limited.
	AllocatedCores float64 `json:"allocatedCores"`
	// UsageSeconds, UserSeconds and SystemSeconds are cumulative CPU time
	// since the container started.
	UsageSeconds  float64 `json:"usageSeconds"`
	UserSeconds   float64 `json:"userSeconds"`
	SystemSeconds float64 `json:"systemSeconds"`
	// Periods counts quota enforcement periods elapsed and
	// ThrottledPeriods those in which the container hit its quota. Both
	// stay 0 without a quota.
	Periods          uint64  `json:"periods"`
	ThrottledPeriods uint64  `json:"throttledPeriods"`
	ThrottledSeconds float64 `json:"throttledSeconds"`
}

// ContainerMemory is a container's memory usage in the requested Unit.
type ContainerMemory struct {
	// Used is the working set: usage minus reclaimable inactive file
	// cache, the same figure docker stats and GetMemory's ContainerAware
	// mode report.
	Used float64 `json:"used"`
	// Limit is the cgroup memory limit when Limited, otherwise the host's
	// total memory - the most the container could actually use.
	Limit          float64 `json:"limit"`
	Limited        bool    `json:"limited"`
	PercentageUsed float64 `json:"percentageUsed"`
	// Anon is anonymous memory (heap, stacks); File is page cache,
	// including the reclaimable part excluded from Used.
	Anon float64 `json:"anon"`
	File float64 `json:"file"`
	// Swap is swap in use by the container, 0 when swap accounting is off.
	Swap float64 `json:"swap"`
	// OOMKills counts processes in the container killed for exceeding the
	// limit. Cumulative, so any increase between polls is worth an alert.
	OOMKills uint64 `json:"oomKills"`
	Unit     Unit   `json:"unit"`
}

// ContainerNetwork holds the counters of the network namespace a
// container's processes are in.
type ContainerNetwork struct {
	// Interfaces excludes loopback.
	Interfaces []ContainerInterface `json:"interfaces"`
	// SharesHostNetwork is true when the container runs in the host's
	// network namespace (docker --network host). Interfaces are then the
	// host's own, so don't add them up across containers.
	SharesHostNetwork bool `json:"sharesHostNetwork"`
	// Accessible is false when /proc/<pid>/net/dev couldn't be read. The
	// container's traffic is unknown, not zero.
	Accessible bool `json:"accessible"`
}

// ContainerInterface holds one interface's cumulative counters inside a
// container's network namespace.
type ContainerInterface struct {
	Interface string `json:"interface"`
	RxBytes   uint64 `json:"rxBytes"`
	TxBytes   uint64 `json:"txBytes"`
	RxPackets uint64 `json:"rxPackets"`
	TxPackets uint64 `json:"txPackets"`
	RxErrors  uint64 `json:"rxErrors"`
	TxErrors  uint64 `json:"txErrors"`
	RxDropped uint64 `json:"rxDropped"`
	TxDropped uint64 `json:"txDropped"`
}

// ContainerBlockIO holds a container's cumulative I/O against one block
// device.
type ContainerBlockIO struct {
	// Device is the kernel's name for the device ("sda", "nvme0n1"),
	// or "major:minor" when it isn't listed in /proc/diskstats.
	Device     string `json:"device"`
	Major      uint64 `json:"major"`
	Minor      uint64 `json:"minor"`
	ReadBytes  uint64 `json:"readBytes"`
	WriteBytes uint64 `json:"writeBytes"`
	Reads      uint64 `json:"reads"`
	Writes     uint64 `json:"writes"`
}

// ContainerMount is one filesystem mounted in a container: the root
// filesystem and any volumes or bind mounts.
//
// The sizes are those of the filesystem being mounted, not of what the
// container has written to it. For the root overlay, that's the host
// filesystem holding the image layers - every container on the host shows
// the same figures for "/". Container.Layer has the container's own
// usage.
type ContainerMount struct {
	MountPoint string  `json:"mountPoint"`
	Device     string  `json:"device"`
	FSType     string  `json:"fsType"`
	Total      float64 `json:"total"`
	Used       float64 `json:"used"`
	Available  float64 `json:"available"`
	Unit       Unit    `json:"unit"`

	Inodes          uint64 `json:"inodes"`
	InodesUsed      uint64 `json:"inodesUsed"`
	InodesAvailable uint64 `json:"inodesAvailable"`
	// Accessible is false when the mount couldn't be statted through
	// /proc/<pid>/root, which needs root (or CAP_SYS_PTRACE). Every size
	// is zero in that case.
	Accessible bool `json:"accessible"`
}

// ContainerPids is the container's task count against its pids limit.
type ContainerPids struct {
	// Current counts tasks - threads included, since that's what the
	// kernel's pids limit counts.
	Current uint64 `json:"current"`
	Max     uint64 `json:"max"`
	Limited bool   `json:"limited"`
}

// ContainerRates holds per-second rates derived from two samples of the
// same container, summed across its interfaces and block devices.
type ContainerRates struct {
	ID                    string  `json:"id"`
	RxBytesPerSec         float64 `json:"rxBytesPerSec"`
	TxBytesPerSec         float64 `json:"txBytesPerSec"`
	RxPacketsPerSec       float64 `json:"rxPacketsPerSec"`
	TxPacketsPerSec       float64 `json:"txPacketsPerSec"`
	ReadBytesPerSec       float64 `json:"readBytesPerSec"`
	WriteBytesPerSec      float64 `json:"writeBytesPerSec"`
	ReadsPerSec           float64 `json:"readsPerSec"`
	WritesPerSec          float64 `json:"writesPerSec"`
	CPUThrottledPerSec    float64 `json:"cpuThrottledPerSec"`
	CPUThrottledSecPerSec float64 `json:"cpuThrottledSecPerSec"`
}

// RatesSince computes per-second rates between prev and c, which must be
// samples of the same container. Network, block I/O and throttling are
// computed independently: if one group's counters went backwards (an
// interface was recreated, a device detached), that group reports zeros
// and the others are unaffected. Everything is zero if the IDs differ or
// elapsedSeconds isn't positive.
func (c Container) RatesSince(prev Container, elapsedSeconds float64) ContainerRates {
	rates := ContainerRates{ID: c.ID}
	if elapsedSeconds <= 0 || c.ID != prev.ID {
		return rates
	}

	now, then := c.Network.totals(), prev.Network.totals()
	if now.RxBytes >= then.RxBytes && now.TxBytes >= then.TxBytes &&
		now.RxPackets >= then.RxPackets && now.TxPackets >= then.TxPackets {
		rates.RxBytesPerSec = float64(now.RxBytes-then.RxBytes) / elapsedSeconds
		rates.TxBytesPerSec = float64(now.TxBytes-then.TxBytes) / elapsedSeconds
		rates.RxPacketsPerSec = float64(now.RxPackets-then.RxPackets) / elapsedSeconds
		rates.TxPacketsPerSec = float64(now.TxPackets-then.TxPackets) / elapsedSeconds
	}

	ioNow, ioThen := blockIOTotals(c.BlockIO), blockIOTotals(prev.BlockIO)
	if ioNow.ReadBytes >= ioThen.ReadBytes && ioNow.WriteBytes >= ioThen.WriteBytes &&
		ioNow.Reads >= ioThen.Reads && ioNow.Writes >= ioThen.Writes {
		rates.ReadBytesPerSec = float64(ioNow.ReadBytes-ioThen.ReadBytes) / elapsedSeconds
		rates.WriteBytesPerSec = float64(ioNow.WriteBytes-ioThen.WriteBytes) / elapsedSeconds
		rates.ReadsPerSec = float64(ioNow.Reads-ioThen.Reads) / elapsedSeconds
		rates.WritesPerSec = float64(ioNow.Writes-ioThen.Writes) / elapsedSeconds
	}

	if c.CPU.ThrottledPeriods >= prev.CPU.ThrottledPeriods && c.CPU.ThrottledSeconds >= prev.CPU.ThrottledSeconds {
		rates.CPUThrottledPerSec = float64(c.CPU.ThrottledPeriods-prev.CPU.ThrottledPeriods) / elapsedSeconds
		rates.CPUThrottledSecPerSec = (c.CPU.ThrottledSeconds - prev.CPU.ThrottledSeconds) / elapsedSeconds
	}

	return rates
}

func (n ContainerNetwork) totals() ContainerInterface {
	var t ContainerInterface
	for _, i := range n.Interfaces {
		t.RxBytes += i.RxBytes
		t.TxBytes += i.TxBytes
		t.RxPackets += i.RxPackets
		t.TxPackets += i.TxPackets
	}
	return t
}

func blockIOTotals(devices []ContainerBlockIO) ContainerBlockIO {
	var t ContainerBlockIO
	for _, d := range devices {
		t.ReadBytes += d.ReadBytes
		t.WriteBytes += d.WriteBytes
		t.Reads += d.Reads
		t.Writes += d.Writes
	}
	return t
}

// containerHostInfo is read once per call and shared by every container:
// the host-wide denominators and lookup tables.
type containerHostInfo struct {
	cores       int
	memoryBytes uint64
	diskNames   map[string]string // "major:minor" -> device name
	netNS       string            // readlink of /proc/1/ns/net
}

func readContainerHostInfo(systats *SyStats) containerHostInfo {
	info := containerHostInfo{diskNames: map[string]string{}}
	if stat, err := fileops.ReadFileWithError(systats.StatFilePath); err == nil {
		info.cores = countStatCPUs(stat)
	}
	if meminfo, err := fileops.ReadFileWithError(systats.MeminfoPath); err == nil {
		info.memoryBytes = parseMeminfo(meminfo).total * 1024
	}
	if diskstats, err := fileops.ReadFileWithError(systats.DiskStatsPath); err == nil {
		info.diskNames = parseDiskNames(diskstats)
	}
	info.netNS, _ = os.Readlink(path.Join(systats.ProcPath, "1", "ns", "net"))
	return info
}

// countStatCPUs counts /proc/stat's per-CPU lines ("cpu0", "cpu1", ...),
// skipping the aggregate "cpu" line.
func countStatCPUs(content string) int {
	n := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "cpu") && len(line) > 3 && line[3] >= '0' && line[3] <= '9' {
			n++
		}
	}
	return n
}

// parseDiskNames maps each /proc/diskstats device to its name. Unlike
// parseDiskStats it keeps loop and ram devices - a container's I/O
// against a loop-mounted image should still be named.
func parseDiskNames(content string) map[string]string {
	names := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		names[f[0]+":"+f[1]] = f[2]
	}
	return names
}

func getContainers(ctx context.Context, systats *SyStats, unit Unit) ([]Container, error) {
	return collectContainers(ctx, systats, unit, "", false)
}

func getContainer(ctx context.Context, systats *SyStats, unit Unit, idOrName string) (Container, error) {
	containers, err := collectContainers(ctx, systats, unit, idOrName, true)
	if err != nil {
		return Container{}, err
	}
	if len(containers) == 0 {
		// It matched during discovery, then stopped before its stats
		// could be read.
		return Container{}, errors.New("container " + idOrName + " stopped while being read")
	}
	return containers[0], nil
}

// collectContainers discovers containers, optionally narrows them to the
// one matching idOrName, and reads their stats. CPU is sampled for every
// container across one shared window, so the call costs one
// CPUSampleWindow however many containers there are.
func collectContainers(ctx context.Context, systats *SyStats, unit Unit, idOrName string, single bool) ([]Container, error) {
	bytesPer, ok := bytesPerUnit[unit]
	if !ok {
		return nil, errors.New(string(unit) + " is not supported")
	}

	version := detectCgroupVersion(systats.CgroupRootPath)
	if version == cgroupNone {
		return nil, errors.New("no cgroup filesystem found at " + systats.CgroupRootPath)
	}

	refs := discoverContainers(systats, version)
	containers := make([]Container, len(refs))
	for i, ref := range refs {
		containers[i] = Container{
			ID:         ref.id,
			ShortID:    shortContainerID(ref.id),
			Runtime:    ref.runtime,
			PodUID:     ref.podUID,
			CgroupPath: ref.relPath,
			Pid:        ref.pid,
		}
	}

	if len(refs) > 0 {
		// Metadata is best-effort: without a socket every container is
		// still reported, just without a name.
		if meta, err := fetchContainerMetadata(ctx, systats.ContainerSocketPath, systats.ContainerSocketTimeout); err == nil {
			for i := range containers {
				if m, ok := meta[containers[i].ID]; ok {
					containers[i].Name = m.name
					containers[i].Image = m.image
					containers[i].State = m.state
					containers[i].Labels = m.labels
					containers[i].MetadataAvailable = true
				}
			}
		}
	}

	if single {
		i, err := matchContainer(containers, idOrName)
		if err != nil {
			return nil, err
		}
		containers, refs = containers[i:i+1], refs[i:i+1]
	}

	host := readContainerHostInfo(systats)

	usage1 := make([]float64, len(refs))
	ok1 := make([]bool, len(refs))
	for i, ref := range refs {
		usage1[i], ok1[i] = sampleContainerCPUSeconds(ref.dirs)
	}
	start := time.Now()
	if err := sleepCtx(ctx, systats.cpuSampleWindow()); err != nil {
		return nil, err
	}
	// All second readings are taken back to back, before any other file
	// is read, so every container's delta spans the same elapsed time.
	usage2 := make([]float64, len(refs))
	ok2 := make([]bool, len(refs))
	for i, ref := range refs {
		usage2[i], ok2[i] = sampleContainerCPUSeconds(ref.dirs)
	}
	elapsed := time.Since(start).Seconds()

	out := make([]Container, 0, len(containers))
	for i := range containers {
		c := containers[i]
		if err := readContainerStats(&c, refs[i], systats, host, unit, bytesPer); err != nil {
			continue // stopped mid-read; its cgroup is gone
		}
		c.Layer = ContainerLayer{Unit: unit}
		if systats.ContainerLayerSize {
			layer, err := readContainerLayer(ctx, systats.ProcPath, refs[i].pid, unit, bytesPer)
			if err != nil {
				return nil, err // only a cancelled ctx gets here
			}
			c.Layer = layer
		}
		applyContainerCPUSample(&c.CPU, usage1[i], usage2[i], ok1[i] && ok2[i], elapsed, host.cores)
		out = append(out, c)
	}
	return out, nil
}

// matchContainer finds idOrName among containers: an exact ID or name
// first, then a unique ID prefix (the 12-character short IDs docker ps
// shows, or any other length).
func matchContainer(containers []Container, idOrName string) (int, error) {
	if idOrName == "" {
		return 0, errors.New("empty container ID or name")
	}
	for i, c := range containers {
		if c.ID == idOrName || (c.Name != "" && c.Name == idOrName) {
			return i, nil
		}
	}
	found := -1
	for i, c := range containers {
		if strings.HasPrefix(c.ID, idOrName) {
			if found >= 0 {
				return 0, fmt.Errorf("container ID prefix %q is ambiguous", idOrName)
			}
			found = i
		}
	}
	if found < 0 {
		return 0, fmt.Errorf("no running container matches %q", idOrName)
	}
	return found, nil
}

func shortContainerID(id string) string {
	if len(id) == 64 {
		return id[:12]
	}
	return id
}

// readContainerStats fills in everything but the CPU sample. It returns
// an error only when the container's memory cgroup can't be read, which in
// practice means it stopped mid-call - every other source degrades to a
// flag or an empty value. Panics from malformed cgroup files (a file that
// vanished between checking and reading parses as "") are recovered here,
// so one container disappearing doesn't fail the whole call.
func readContainerStats(c *Container, ref containerRef, systats *SyStats, host containerHostInfo, unit Unit, bytesPer float64) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("reading container %s: %v", c.ID, r)
		}
	}()

	dirs := ref.dirs
	c.Time = time.Now().Unix()

	mem, err := readContainerMemory(dirs, host.memoryBytes, bytesPer)
	if err != nil {
		return err
	}
	mem.Unit = unit
	c.Memory = mem

	c.CPU = readContainerCPUCounters(dirs)
	c.Pids = readContainerPids(dirs.pids)
	c.Pressure = readContainerPressure(dirs)
	c.BlockIO = readContainerBlockIO(dirs, host.diskNames)
	c.Network = readContainerNetwork(systats.ProcPath, ref.pid, host.netNS)
	c.Mounts = readContainerMounts(systats.ProcPath, ref.pid, unit, bytesPer)

	if c.State == "" {
		c.State = "running"
		if isCgroupFrozen(dirs) {
			c.State = "paused"
		}
	}
	return nil
}

func readContainerMemory(dirs cgroupDirs, hostMemoryBytes uint64, bytesPer float64) (ContainerMemory, error) {
	limitBytes, limited, usedBytes, err := readCgroupMemoryBytes(dirs.version, dirs.memory)
	if err != nil {
		return ContainerMemory{}, err
	}
	if !limited {
		limitBytes = hostMemoryBytes
	}

	m := ContainerMemory{
		Used:    float64(usedBytes) / bytesPer,
		Limit:   float64(limitBytes) / bytesPer,
		Limited: limited,
	}
	if limitBytes > 0 {
		m.PercentageUsed = float64(usedBytes) / float64(limitBytes) * 100
	}

	stat := readKeyValueStat(path.Join(dirs.memory, "memory.stat"))
	var anon, file, swap, oomKills uint64
	if dirs.version == cgroupV2 {
		anon, file = stat["anon"], stat["file"]
		swap = readUintFile(path.Join(dirs.memory, "memory.swap.current"))
		oomKills = readKeyValueStat(path.Join(dirs.memory, "memory.events"))["oom_kill"]
	} else {
		anon, file = statWithFallback(stat, "total_rss", "rss"), statWithFallback(stat, "total_cache", "cache")
		// memsw is memory+swap; only present with swap accounting enabled.
		if memsw := readUintFile(path.Join(dirs.memory, "memory.memsw.usage_in_bytes")); memsw > 0 {
			if usage, err := readCgroupMemoryUsage(dirs.version, dirs.memory); err == nil && memsw > usage {
				swap = memsw - usage
			}
		}
		oomKills = readKeyValueStat(path.Join(dirs.memory, "memory.oom_control"))["oom_kill"]
	}
	m.Anon = float64(anon) / bytesPer
	m.File = float64(file) / bytesPer
	m.Swap = float64(swap) / bytesPer
	m.OOMKills = oomKills
	return m, nil
}

// statWithFallback prefers v1's hierarchical total_* key, which includes
// child cgroups, over the local one.
func statWithFallback(stat map[string]uint64, key, fallback string) uint64 {
	if v, ok := stat[key]; ok {
		return v
	}
	return stat[fallback]
}

// readUintFile reads a single-number cgroup file, 0 when it's missing or
// isn't a number (including v2's "max").
func readUintFile(file string) uint64 {
	content, err := os.ReadFile(file)
	if err != nil {
		return 0
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(content)), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// sampleContainerCPUSeconds reads the container's cumulative CPU usage
// counter in seconds, reusing the same reader as ContainerAware GetCPU.
func sampleContainerCPUSeconds(dirs cgroupDirs) (float64, bool) {
	return sampleCgroupCPUUsageSeconds(cgroupCPUInfo{ok: true, version: dirs.version, usageDir: dirs.cpuacct})
}

// readContainerCPUCounters reads the quota and the cumulative counters. The
// sampled fields (CoresUsed and the percentages) are filled in afterwards
// by applyContainerCPUSample.
func readContainerCPUCounters(dirs cgroupDirs) ContainerCPU {
	out := ContainerCPU{}
	if cores, limited, err := readCgroupCPUQuota(dirs.version, dirs.cpu); err == nil && limited {
		out.Limited = true
		out.AllocatedCores = cores
	}

	if dirs.version == cgroupV2 {
		stat := readKeyValueStat(path.Join(dirs.cpu, "cpu.stat"))
		out.UsageSeconds = float64(stat["usage_usec"]) / 1e6
		out.UserSeconds = float64(stat["user_usec"]) / 1e6
		out.SystemSeconds = float64(stat["system_usec"]) / 1e6
		out.Periods = stat["nr_periods"]
		out.ThrottledPeriods = stat["nr_throttled"]
		out.ThrottledSeconds = float64(stat["throttled_usec"]) / 1e6
		return out
	}

	if usage, ok := sampleContainerCPUSeconds(dirs); ok {
		out.UsageSeconds = usage
	}
	// cpuacct.stat is in USER_HZ ticks.
	acct := readKeyValueStat(path.Join(dirs.cpuacct, "cpuacct.stat"))
	out.UserSeconds = float64(acct["user"]) / clockTicksPerSec
	out.SystemSeconds = float64(acct["system"]) / clockTicksPerSec
	stat := readKeyValueStat(path.Join(dirs.cpu, "cpu.stat"))
	out.Periods = stat["nr_periods"]
	out.ThrottledPeriods = stat["nr_throttled"]
	out.ThrottledSeconds = float64(stat["throttled_time"]) / 1e9
	return out
}

// applyContainerCPUSample turns two usage readings into CoresUsed and the
// percentages. A counter that went backwards (the cgroup was recreated
// under the same name) or a failed read leaves them at zero.
func applyContainerCPUSample(out *ContainerCPU, usage1, usage2 float64, ok bool, elapsedSeconds float64, hostCores int) {
	if usage2 > 0 {
		out.UsageSeconds = usage2
	}
	if !ok || elapsedSeconds <= 0 || usage2 < usage1 {
		return
	}
	out.CoresUsed = (usage2 - usage1) / elapsedSeconds
	if hostCores > 0 {
		out.PercentOfHost = 100 * out.CoresUsed / float64(hostCores)
	}
	if out.Limited && out.AllocatedCores > 0 {
		out.PercentOfLimit = 100 * out.CoresUsed / out.AllocatedCores
	}
}

func readContainerPids(dir string) ContainerPids {
	out := ContainerPids{Current: readUintFile(path.Join(dir, "pids.current"))}
	if limit := readUintFile(path.Join(dir, "pids.max")); limit > 0 {
		out.Max = limit
		out.Limited = true
	}
	return out
}

// readContainerPressure reads the container's own PSI files. Only cgroup
// v2 has them.
func readContainerPressure(dirs cgroupDirs) Pressure {
	out := Pressure{Time: time.Now().Unix()}
	if dirs.version != cgroupV2 {
		return out
	}
	for _, r := range []psiResource{{"cpu", &out.CPU}, {"memory", &out.Memory}, {"io", &out.IO}} {
		content, err := fileops.ReadFileWithError(pressureFilePath(dirs.cpu, r.name, true))
		if err != nil {
			continue
		}
		*r.target = parsePressure(content)
		out.Available = true
	}
	out.Limited = out.Available
	return out
}

func isCgroupFrozen(dirs cgroupDirs) bool {
	if dirs.version == cgroupV2 {
		return readKeyValueStat(path.Join(dirs.cpu, "cgroup.events"))["frozen"] == 1
	}
	return strings.TrimSpace(fileops.ReadFile(path.Join(dirs.freezer, "freezer.state"))) == "FROZEN"
}

func readContainerBlockIO(dirs cgroupDirs, diskNames map[string]string) []ContainerBlockIO {
	var devices []ContainerBlockIO
	if dirs.version == cgroupV2 {
		devices = parseIOStat(fileops.ReadFile(path.Join(dirs.blkio, "io.stat")))
	} else {
		devices = parseBlkioThrottle(
			readFirstExisting(dirs.blkio, "blkio.throttle.io_service_bytes_recursive", "blkio.throttle.io_service_bytes"),
			readFirstExisting(dirs.blkio, "blkio.throttle.io_serviced_recursive", "blkio.throttle.io_serviced"),
		)
	}
	for i := range devices {
		key := fmt.Sprintf("%d:%d", devices[i].Major, devices[i].Minor)
		if name, ok := diskNames[key]; ok {
			devices[i].Device = name
		} else {
			devices[i].Device = key
		}
	}
	return devices
}

func readFirstExisting(dir string, names ...string) string {
	for _, n := range names {
		if content, err := fileops.ReadFileWithError(path.Join(dir, n)); err == nil {
			return content
		}
	}
	return ""
}

// parseIOStat parses cgroup v2 io.stat:
//
//	8:0 rbytes=1459200 wbytes=314773504 rios=192 wios=353 dbytes=0 dios=0
func parseIOStat(content string) []ContainerBlockIO {
	out := []ContainerBlockIO{}
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		major, minor, ok := parseDevNum(fields[0])
		if !ok {
			continue
		}
		d := ContainerBlockIO{Major: major, Minor: minor}
		for _, kv := range fields[1:] {
			key, value, found := strings.Cut(kv, "=")
			if !found {
				continue
			}
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				continue
			}
			switch key {
			case "rbytes":
				d.ReadBytes = n
			case "wbytes":
				d.WriteBytes = n
			case "rios":
				d.Reads = n
			case "wios":
				d.Writes = n
			}
		}
		out = append(out, d)
	}
	return out
}

// parseBlkioThrottle merges cgroup v1's io_service_bytes and io_serviced,
// which share a format - one "major:minor Op value" line per operation,
// and a trailing "Total value" line:
//
//	8:0 Read 1459200
//	8:0 Write 314773504
//	...
//	Total 316232704
func parseBlkioThrottle(bytesContent, opsContent string) []ContainerBlockIO {
	byDev := map[string]*ContainerBlockIO{}
	order := []string{}
	get := func(dev string) *ContainerBlockIO {
		if d, ok := byDev[dev]; ok {
			return d
		}
		major, minor, ok := parseDevNum(dev)
		if !ok {
			return nil
		}
		d := &ContainerBlockIO{Major: major, Minor: minor}
		byDev[dev] = d
		order = append(order, dev)
		return d
	}

	apply := func(content string, read, write func(*ContainerBlockIO, uint64)) {
		for _, line := range strings.Split(content, "\n") {
			fields := strings.Fields(line)
			if len(fields) != 3 {
				continue
			}
			n, err := strconv.ParseUint(fields[2], 10, 64)
			if err != nil {
				continue
			}
			d := get(fields[0])
			if d == nil {
				continue
			}
			switch fields[1] {
			case "Read":
				read(d, n)
			case "Write":
				write(d, n)
			}
		}
	}
	apply(bytesContent,
		func(d *ContainerBlockIO, n uint64) { d.ReadBytes = n },
		func(d *ContainerBlockIO, n uint64) { d.WriteBytes = n })
	apply(opsContent,
		func(d *ContainerBlockIO, n uint64) { d.Reads = n },
		func(d *ContainerBlockIO, n uint64) { d.Writes = n })

	out := make([]ContainerBlockIO, 0, len(order))
	for _, dev := range order {
		out = append(out, *byDev[dev])
	}
	return out
}

func parseDevNum(s string) (major, minor uint64, ok bool) {
	a, b, found := strings.Cut(s, ":")
	if !found {
		return 0, 0, false
	}
	major, err1 := strconv.ParseUint(a, 10, 64)
	minor, err2 := strconv.ParseUint(b, 10, 64)
	return major, minor, err1 == nil && err2 == nil
}

// readContainerNetwork reads the counters of the network namespace pid is
// in. /proc/<pid>/net is namespace-relative, so this sees the container's
// interfaces without having to enter the namespace.
func readContainerNetwork(procPath string, pid int, hostNetNS string) ContainerNetwork {
	pidDir := path.Join(procPath, strconv.Itoa(pid))
	content, err := fileops.ReadFileWithError(path.Join(pidDir, "net", "dev"))
	if err != nil {
		return ContainerNetwork{}
	}
	out := ContainerNetwork{Interfaces: parseNetDev(content), Accessible: true}
	if hostNetNS != "" {
		if ns, err := os.Readlink(path.Join(pidDir, "ns", "net")); err == nil && ns == hostNetNS {
			out.SharesHostNetwork = true
		}
	}
	return out
}

// parseNetDev parses /proc/net/dev, skipping loopback and the two header
// lines:
//
//	eth0: 1296 16 0 0 0 0 0 0 780 10 0 0 0 0 0 0
//
// Receive counters come first (bytes, packets, errs, drop, ...), transmit
// counters start at the ninth column.
func parseNetDev(content string) []ContainerInterface {
	out := []ContainerInterface{}
	for _, line := range strings.Split(content, "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		f := strings.Fields(rest)
		if name == "lo" || len(f) < 12 {
			continue
		}
		n := func(i int) uint64 {
			v, _ := strconv.ParseUint(f[i], 10, 64)
			return v
		}
		out = append(out, ContainerInterface{
			Interface: name,
			RxBytes:   n(0), RxPackets: n(1), RxErrors: n(2), RxDropped: n(3),
			TxBytes: n(8), TxPackets: n(9), TxErrors: n(10), TxDropped: n(11),
		})
	}
	return out
}

// containerPseudoFsTypes are kernel and runtime plumbing mounted into every
// container - never storage anyone asked for. tmpfs and devtmpfs are
// excluded for the same reason GetDisks excludes them.
var containerPseudoFsTypes = map[string]bool{
	"proc": true, "sysfs": true, "cgroup": true, "cgroup2": true,
	"devpts": true, "mqueue": true, "securityfs": true, "debugfs": true,
	"tracefs": true, "pstore": true, "bpf": true, "fusectl": true,
	"configfs": true, "hugetlbfs": true, "autofs": true, "binfmt_misc": true,
	"nsfs": true, "rpc_pipefs": true,
}

// containerRuntimeFileMounts are single files the runtime bind-mounts into
// every container. They sit on the host's root filesystem and would
// otherwise show up as three copies of it.
var containerRuntimeFileMounts = map[string]bool{
	"/etc/hosts": true, "/etc/hostname": true, "/etc/resolv.conf": true,
	"/run/.containerenv": true, "/.dockerenv": true,
}

// isContainerStorageMount reports whether a mount inside a container is
// storage worth reporting: the root filesystem, volumes and bind mounts,
// but not /proc, /sys, /dev or the runtime's injected files.
func isContainerStorageMount(m mountEntry) bool {
	if containerPseudoFsTypes[m.fsType] || excludedFsTypes[m.fsType] || containerRuntimeFileMounts[m.mountPoint] {
		return false
	}
	for _, prefix := range []string{"/proc", "/sys", "/dev"} {
		if m.mountPoint == prefix || strings.HasPrefix(m.mountPoint, prefix+"/") {
			return false
		}
	}
	return true
}

// readContainerMounts lists pid's storage mounts and their usage, statting
// each one through /proc/<pid>/root so the path resolves inside the
// container's mount namespace.
func readContainerMounts(procPath string, pid int, unit Unit, bytesPer float64) []ContainerMount {
	pidDir := path.Join(procPath, strconv.Itoa(pid))
	mounts, err := readMounts(path.Join(pidDir, "mounts"))
	if err != nil {
		return []ContainerMount{}
	}

	out := []ContainerMount{}
	for _, m := range mounts {
		if !isContainerStorageMount(m) {
			continue
		}
		cm := ContainerMount{MountPoint: m.mountPoint, Device: m.device, FSType: m.fsType, Unit: unit}

		var stat unix.Statfs_t
		if err := unix.Statfs(path.Join(pidDir, "root", m.mountPoint), &stat); err == nil {
			if stat.Blocks == 0 {
				continue // a pseudo filesystem the list above doesn't know
			}
			blockSize := uint64(stat.Bsize)
			size := uint64(stat.Blocks) * blockSize
			free := uint64(stat.Bfree) * blockSize
			cm.Total = float64(size) / bytesPer
			cm.Used = float64(size-free) / bytesPer
			cm.Available = float64(uint64(stat.Bavail)*blockSize) / bytesPer
			cm.Inodes = uint64(stat.Files)
			cm.InodesAvailable = uint64(stat.Ffree)
			cm.InodesUsed = cm.Inodes - cm.InodesAvailable
			cm.Accessible = true
		}
		out = append(out, cm)
	}
	return out
}
