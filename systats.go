package systats

import "fmt"

const (
	Byte     string = "B"
	Kilobyte string = "KB"
	Megabyte string = "MB"
	Gigabyte string = "GB"

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
	StatFilePath    string
	CPUinfoFilePath string
	VersionPath     string
	EtcPath         string
	UptimePath      string
	MountsPath      string
	LoadAvgPath     string
	// ProcessCPUMode selects how GetTopProcesses computes CPU usage:
	// CPUUsageInstant (default, used when left empty) or CPUUsageAverage.
	ProcessCPUMode string
}

func New() SyStats {
	return SyStats{
		MeminfoPath:     "/proc/meminfo",
		StatFilePath:    "/proc/stat",
		CPUinfoFilePath: "/proc/cpuinfo",
		VersionPath:     "/proc/version",
		EtcPath:         "/etc/",
		UptimePath:      "/proc/uptime",
		MountsPath:      "/proc/mounts",
		LoadAvgPath:     "/proc/loadavg",
		ProcessCPUMode:  CPUUsageInstant,
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

func (systats *SyStats) GetDisks() ([]Disk, error) {
	return withRecover(func() ([]Disk, error) { return getDisks(systats) })
}

func (systats *SyStats) IsPortOpen(port int) bool {
	return isPortOpen(port)
}

func (systats *SyStats) CanConnectExternal(url string) (bool, error) {
	return withRecover(func() (bool, error) { return canConnect(url) })
}

func (systats *SyStats) EstablishedTCPConnCount(procName string) int {
	return establishedTCPConnCount(procName)
}
