package systats

import "fmt"

const (
	// Unit constants for GetMemory, GetSwap and Disk.Convert. These are
	// binary units despite the names - Kilobyte is KiB, Megabyte is MiB,
	// Gigabyte is GiB - matching what free(1), df(1) and top(1) report.
	Byte     string = "B"
	Kilobyte string = "KB"
	Megabyte string = "MB"
	Gigabyte string = "GB"

	// Sort orders for GetTopProcesses.
	SortByCPU    string = "cpu"
	SortByMemory string = "memory"

	// CPUUsageInstant (default) computes each process's CPU usage over a
	// short live sampling window (like `top`) - correct for monitoring
	// "what's using CPU right now", but GetTopProcesses pays at least
	// the sampling window (300ms) in latency on every call.
	CPUUsageInstant string = "instant"
	// CPUUsageAverage computes each process's CPU usage as a lifetime
	// average since it started (total CPU time / time since start),
	// matching `ps`'s default %cpu. A single /proc read per process, no
	// sampling wait - much faster, but can miss a process that's
	// currently spiking after being idle for a long time.
	CPUUsageAverage string = "average"
)

// SyStats holds information used to collect data
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
	// ProcessCPUMode selects how GetTopProcesses computes CPU usage:
	// CPUUsageInstant (default, used when left empty) or CPUUsageAverage.
	ProcessCPUMode string
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
		ProcessCPUMode:  CPUUsageInstant,
		ContainerAware:  false,
		CgroupRootPath:  "/sys/fs/cgroup",
		SelfCgroupPath:  "/proc/self/cgroup",
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

func (systats *SyStats) GetMemory(unit string) (Memory, error) {
	return withRecover(func() (Memory, error) { return getMemory(systats, unit) })
}

func (systats *SyStats) GetSwap(unit string) (Swap, error) {
	return withRecover(func() (Swap, error) { return getSwap(systats, unit) })
}

func (systats *SyStats) GetCPU() (CPU, error) {
	return withRecover(func() (CPU, error) { return getCPU(systats, 300) })
}

func (systats *SyStats) GetSystem() (System, error) {
	return withRecover(func() (System, error) { return getSystem(systats) })
}

func (systats *SyStats) GetNetworks() ([]Network, error) {
	return withRecover(func() ([]Network, error) { return getNetworks() })
}

func (systats *SyStats) GetNetworkUsage(networkInterface string) NetworkUsage {
	return getNetworkUsage(networkInterface)
}

func (systats *SyStats) IsServiceRunning(service string) bool {
	return isServiceRunning(service)
}

func (systats *SyStats) GetTopProcesses(count int, sort string) ([]Process, error) {
	return withRecover(func() ([]Process, error) { return getTopProcesses(systats, count, sort) })
}

// GetProcess looks up a single process by pid, returning an error if it
// doesn't exist or can't be read. Like GetTopProcesses, it honors
// ProcessCPUMode - which means the default instant mode costs a ~300ms
// sampling window per call.
func (systats *SyStats) GetProcess(pid int) (Process, error) {
	return withRecover(func() (Process, error) { return getProcess(systats, pid) })
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
	return isPortOpen(port)
}

func (systats *SyStats) CanConnectExternal(url string) (bool, error) {
	return withRecover(func() (bool, error) { return canConnect(url) })
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

// GetProtocolStats returns per-protocol network counters from
// /proc/net/snmp and /proc/net/netstat - TCP retransmits, UDP errors,
// listen overflows and so on.
func (systats *SyStats) GetProtocolStats() (ProtocolStats, error) {
	return withRecover(func() (ProtocolStats, error) { return getProtocolStats(systats) })
}
