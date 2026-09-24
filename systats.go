package systats

import (
	"context"
	"fmt"
	"time"
)

// defaultCPUSampleWindow is how long the two-sample CPU delta spans when
// SyStats.CPUSampleWindow is left at its zero value.
const defaultCPUSampleWindow = 300 * time.Millisecond

// Unit is a size unit accepted by GetMemory, GetSwap and Disk.Convert.
// It is a defined type so a misspelled unit is a compile error rather
// than a runtime one; untyped constants like "MB" still work.
type Unit string

// Unit constants. These are binary units despite the names - Kilobyte is
// KiB, Megabyte is MiB, Gigabyte is GiB - matching what free(1), df(1)
// and top(1) report.
const (
	Byte     Unit = "B"
	Kilobyte Unit = "KB"
	Megabyte Unit = "MB"
	Gigabyte Unit = "GB"
)

// SortOrder is how GetTopProcesses ranks processes.
type SortOrder string

const (
	SortByCPU    SortOrder = "cpu"
	SortByMemory SortOrder = "memory"
)

// CPUMode selects how per-process CPU usage is calculated.
type CPUMode string

const (
	// CPUUsageInstant (default) computes each process's CPU usage over a
	// short live sampling window (like `top`) - correct for monitoring
	// "what's using CPU right now", but GetTopProcesses pays at least
	// the sampling window (300ms) in latency on every call.
	CPUUsageInstant CPUMode = "instant"
	// CPUUsageAverage computes each process's CPU usage as a lifetime
	// average since it started (total CPU time / time since start),
	// matching `ps`'s default %cpu. A single /proc read per process, no
	// sampling wait - much faster, but can miss a process that's
	// currently spiking after being idle for a long time.
	CPUUsageAverage CPUMode = "average"
)

// SyStats holds information used to collect data.
//
// Safe for concurrent use once configured: no method writes to its
// receiver, so goroutines may share one value freely. The fields below
// are plain struct fields with no synchronization, so set them before the
// first call - mutating one while another goroutine is inside a method is
// a data race.
type SyStats struct {
	MeminfoPath     string
	ProcPath        string
	StatFilePath    string
	CPUinfoFilePath string
	VersionPath     string
	EtcPath         string
	UptimePath      string
	MountsPath      string
	LoadAvgPath     string
	DiskStatsPath   string
	NetTCPPath      string
	NetTCP6Path     string
	NetSNMPPath     string
	NetNetstatPath  string
	// SysClassNetPath is the sysfs directory holding per-interface
	// state and Rx/Tx counters.
	SysClassNetPath string
	// PressurePath is the directory holding the kernel's Pressure Stall
	// Information files (cpu, memory, io).
	PressurePath string
	// HwmonPath is the sysfs directory holding hardware monitoring chips
	// and their temperature sensors.
	HwmonPath string
	// ProcessCPUMode selects how GetTopProcesses computes CPU usage:
	// CPUUsageInstant (default, used when left empty) or CPUUsageAverage.
	ProcessCPUMode CPUMode
	// ContainerAware, when true, makes GetMemory and GetCPU look for a
	// cgroup (v1 or v2) memory/CPU limit applying to the calling process
	// and report container-relative numbers instead of host-wide
	// /proc/meminfo and /proc/stat numbers (see Memory.Limited/
	// CPU.Limited). This also applies under a plain systemd-managed
	// cgroup with MemoryMax=/CPUQuota= set, not just inside a container.
	// Defaults to false: today's host-wide behavior, unchanged.
	ContainerAware bool
	// CgroupRootPath is the mount point of the cgroup filesystem.
	CgroupRootPath string
	// SelfCgroupPath is the file listing which cgroup(s) the calling
	// process belongs to.
	SelfCgroupPath string
	// CPUSampleWindow is how far apart the two samples are taken when
	// computing CPU utilization - by GetCPU always, and by GetTopProcesses
	// and GetProcess in CPUUsageInstant mode. It is the dominant cost of
	// those calls, so shortening it trades accuracy for latency. Zero means
	// defaultCPUSampleWindow (300ms), so a hand-constructed SyStats{} does
	// not end up sampling over no time at all.
	CPUSampleWindow time.Duration
	// ContainerSocketPath is a Docker-compatible API socket that
	// GetContainers asks for container names, images, states and labels.
	// Podman's is /run/podman/podman.sock. Empty disables the lookup;
	// containers are still found and measured from their cgroups either
	// way, just without names.
	ContainerSocketPath string
	// ContainerSocketTimeout bounds the metadata request. Zero means
	// 2 seconds.
	ContainerSocketTimeout time.Duration
	// ContainerLayerSize makes GetContainers measure each container's
	// writable layer (Container.Layer) - the disk it has actually used, as
	// `docker ps -s` shows. Off by default: it walks every file the
	// container has written, which can take seconds for a busy one. The
	// walk honors the context passed to GetContainersWithContext.
	ContainerLayerSize bool
}

// cpuSampleWindow resolves the configured sampling window, falling back to
// the default for a zero (or nonsensical negative) value.
func (systats *SyStats) cpuSampleWindow() time.Duration {
	if systats.CPUSampleWindow <= 0 {
		return defaultCPUSampleWindow
	}
	return systats.CPUSampleWindow
}

func New() SyStats {
	return SyStats{
		MeminfoPath:     "/proc/meminfo",
		ProcPath:        "/proc",
		StatFilePath:    "/proc/stat",
		CPUinfoFilePath: "/proc/cpuinfo",
		VersionPath:     "/proc/version",
		EtcPath:         "/etc/",
		UptimePath:      "/proc/uptime",
		MountsPath:      "/proc/mounts",
		LoadAvgPath:     "/proc/loadavg",
		DiskStatsPath:   "/proc/diskstats",
		NetTCPPath:      "/proc/net/tcp",
		NetTCP6Path:     "/proc/net/tcp6",
		NetSNMPPath:     "/proc/net/snmp",
		NetNetstatPath:  "/proc/net/netstat",
		SysClassNetPath: "/sys/class/net",
		PressurePath:    "/proc/pressure",
		HwmonPath:       "/sys/class/hwmon",
		ProcessCPUMode:  CPUUsageInstant,
		ContainerAware:  false,
		CgroupRootPath:  "/sys/fs/cgroup",
		SelfCgroupPath:  "/proc/self/cgroup",
		CPUSampleWindow: defaultCPUSampleWindow,

		ContainerSocketPath:    "/var/run/docker.sock",
		ContainerSocketTimeout: defaultContainerSocketTimeout,
	}
}

// sleepCtx waits for d, or returns ctx's error as soon as it is cancelled.
// This is what makes the CPU sampling window interruptible: a plain
// time.Sleep would hold the goroutine for the full window even after the
// caller has gone away.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// withRecover converts any panic raised while calling fn into a returned
// error, instead of crashing the caller. This is the defensive boundary
// for internal helpers (e.g. strops.ToUint64/ToFloat64) that panic on
// unexpected /proc content rather than returning an error themselves.
func withRecover[T any](fn func() (T, error)) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("systats: recovered from panic: %v", r)
		}
	}()
	return fn()
}

func (systats *SyStats) GetMemory(unit Unit) (Memory, error) {
	return withRecover(func() (Memory, error) { return getMemory(systats, unit) })
}

func (systats *SyStats) GetSwap(unit Unit) (Swap, error) {
	return withRecover(func() (Swap, error) { return getSwap(systats, unit) })
}

func (systats *SyStats) GetCPU() (CPU, error) {
	return systats.GetCPUWithContext(context.Background())
}

// GetCPUWithContext is GetCPU, abortable via ctx. GetCPU blocks for
// CPUSampleWindow (300ms by default) while it takes its second sample;
// cancelling ctx returns early instead of waiting that out.
func (systats *SyStats) GetCPUWithContext(ctx context.Context) (CPU, error) {
	return withRecover(func() (CPU, error) { return getCPU(ctx, systats) })
}

func (systats *SyStats) GetSystem() (System, error) {
	return systats.GetSystemWithContext(context.Background())
}

// GetSystemWithContext is GetSystem, abortable via ctx. GetSystem shells out
// to who(1) to list logged-in users.
func (systats *SyStats) GetSystemWithContext(ctx context.Context) (System, error) {
	return withRecover(func() (System, error) { return getSystem(ctx, systats) })
}

func (systats *SyStats) GetNetworks() ([]Network, error) {
	return withRecover(func() ([]Network, error) { return getNetworks(systats) })
}

func (systats *SyStats) GetNetworkUsage(networkInterface string) NetworkUsage {
	return getNetworkUsage(systats, networkInterface)
}

// IsServiceRunning reports whether the service is active. It cannot
// distinguish a stopped service from a failed check - use
// IsServiceRunningWithContext if that difference matters.
func (systats *SyStats) IsServiceRunning(service string) bool {
	running, _ := systats.IsServiceRunningWithContext(context.Background(), service)
	return running
}

// IsServiceRunningWithContext is IsServiceRunning, abortable via ctx, and
// returning the error that IsServiceRunning discards. This matters: the
// check shells out to systemctl, so (false, nil) means the service really
// is stopped, while (false, err) means the check itself failed - a wedged
// systemd, a cancelled context, or no systemctl/service binary at all.
func (systats *SyStats) IsServiceRunningWithContext(ctx context.Context, service string) (bool, error) {
	return isServiceRunning(ctx, service)
}

func (systats *SyStats) GetTopProcesses(count int, sort SortOrder) ([]Process, error) {
	return systats.GetTopProcessesWithContext(context.Background(), count, sort)
}

// GetTopProcessesWithContext is GetTopProcesses, abortable via ctx. In the
// default CPUUsageInstant mode this blocks for CPUSampleWindow and then
// walks every pid in /proc, so it is the call most worth bounding.
func (systats *SyStats) GetTopProcessesWithContext(ctx context.Context, count int, sort SortOrder) ([]Process, error) {
	return withRecover(func() ([]Process, error) { return getTopProcesses(ctx, systats, count, sort) })
}

// GetProcess looks up a single process by pid, returning an error if it
// doesn't exist or can't be read. Like GetTopProcesses, it honors
// ProcessCPUMode - which means the default instant mode costs a ~300ms
// sampling window per call.
func (systats *SyStats) GetProcess(pid int) (Process, error) {
	return systats.GetProcessWithContext(context.Background(), pid)
}

// GetProcessWithContext is GetProcess, abortable via ctx - which in the
// default instant mode means not waiting out the sampling window.
func (systats *SyStats) GetProcessWithContext(ctx context.Context, pid int) (Process, error) {
	return withRecover(func() (Process, error) { return getProcess(ctx, systats, pid) })
}

func (systats *SyStats) GetDisks() ([]Disk, error) {
	return withRecover(func() ([]Disk, error) { return getDisks(systats) })
}

// GetDiskIO returns cumulative I/O counters per block device. The values
// are counters since boot, not rates - poll twice and use
// DiskIO.RatesSince to derive throughput, IOPS and utilization.
func (systats *SyStats) GetDiskIO() ([]DiskIO, error) {
	return withRecover(func() ([]DiskIO, error) { return getDiskIO(systats) })
}

func (systats *SyStats) IsPortOpen(port int) bool {
	return systats.IsPortOpenWithContext(context.Background(), port)
}

// IsPortOpenWithContext is IsPortOpen, abortable via ctx. A cancelled
// context reports the port as closed, since a probe that did not complete
// is not evidence that anything is listening.
func (systats *SyStats) IsPortOpenWithContext(ctx context.Context, port int) bool {
	return isPortOpen(ctx, port)
}

func (systats *SyStats) CanConnectExternal(url string) (bool, error) {
	return systats.CanConnectExternalWithContext(context.Background(), url)
}

// CanConnectExternalWithContext is CanConnectExternal, abortable via ctx.
// The 10s client timeout still applies as a backstop when ctx has no
// deadline of its own.
func (systats *SyStats) CanConnectExternalWithContext(ctx context.Context, url string) (bool, error) {
	return withRecover(func() (bool, error) { return canConnect(ctx, url) })
}

func (systats *SyStats) EstablishedTCPConnCount(procName string) int {
	return establishedTCPConnCount(systats, procName)
}

// GetTCPConnectionStates counts every TCP socket in the caller's network
// namespace by connection state, across IPv4 and IPv6. Useful for
// spotting TIME_WAIT buildup or listen-queue problems that a per-process
// count won't show.
func (systats *SyStats) GetTCPConnectionStates() (TCPStates, error) {
	return withRecover(func() (TCPStates, error) { return getTCPConnectionStates(systats) })
}

// GetPressure returns Pressure Stall Information from /proc/pressure:
// how much time tasks spent stalled waiting on CPU, memory and I/O.
//
// This is the metric that answers "is this box saturated" - unlike load
// average or utilization, which can't distinguish a machine that's busy
// from one that's thrashing. Needs kernel 4.20+ with CONFIG_PSI=y; check
// Pressure.Available rather than reading zeros as real, since an older
// kernel is a normal condition rather than an error.
//
// With ContainerAware set, reports the calling process's own cgroup v2
// pressure instead of the host's (cgroup v1 has no PSI equivalent, and
// falls back to host-wide).
func (systats *SyStats) GetPressure() (Pressure, error) {
	return withRecover(func() (Pressure, error) { return getPressure(systats) })
}

// GetTemperatures returns temperature sensor readings from
// /sys/class/hwmon - CPU package and core temps, and whatever else the
// board exposes.
//
// A host with no hwmon chips (most VMs and containers) returns an empty
// slice rather than an error. Sensors that don't publish a high or
// critical threshold set HighAvailable/CriticalAvailable false; treating
// those zeros as real thresholds would make every reading look
// over-limit.
func (systats *SyStats) GetTemperatures() ([]Temperature, error) {
	return withRecover(func() ([]Temperature, error) { return getTemperatures(systats) })
}

// GetProtocolStats returns per-protocol network counters from
// /proc/net/snmp and /proc/net/netstat - TCP retransmits, UDP errors,
// listen overflows and so on.
func (systats *SyStats) GetProtocolStats() (ProtocolStats, error) {
	return withRecover(func() (ProtocolStats, error) { return getProtocolStats(systats) })
}

// GetContainers returns every running container on this host - Docker,
// Podman, containerd/CRI-O under Kubernetes, LXC and systemd-machined -
// with its CPU, memory, network, block I/O, mount, pid and pressure
// stats. Containers are found by walking the cgroup tree, so this works
// without any runtime daemon; names, images and labels are added when
// ContainerSocketPath answers (see Container.MetadataAvailable).
//
// It blocks for CPUSampleWindow once, however many containers there are.
// Run it on the host, or in a container that can see the host's cgroup
// tree and /proc (--pid=host). Mount usage needs root.
//
// A host with no containers returns an empty slice. A host with no cgroup
// filesystem at all returns an error.
func (systats *SyStats) GetContainers(unit Unit) ([]Container, error) {
	return systats.GetContainersWithContext(context.Background(), unit)
}

// GetContainersWithContext is GetContainers, abortable via ctx - both the
// CPU sampling window and the runtime API request.
func (systats *SyStats) GetContainersWithContext(ctx context.Context, unit Unit) ([]Container, error) {
	return withRecover(func() ([]Container, error) { return getContainers(ctx, systats, unit) })
}

// GetContainer returns one running container by full ID, name, or unique
// ID prefix (such as the 12-character IDs docker ps shows). Names need
// ContainerSocketPath to be reachable. Returns an error when nothing
// matches or a prefix matches more than one container.
func (systats *SyStats) GetContainer(unit Unit, idOrName string) (Container, error) {
	return systats.GetContainerWithContext(context.Background(), unit, idOrName)
}

// GetContainerWithContext is GetContainer, abortable via ctx.
func (systats *SyStats) GetContainerWithContext(ctx context.Context, unit Unit, idOrName string) (Container, error) {
	return withRecover(func() (Container, error) { return getContainer(ctx, systats, unit, idOrName) })
}
