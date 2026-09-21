package systats

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
		ProcessCPUMode:  CPUUsageInstant,
	}
}

func (systats *SyStats) GetMemory(unit string) (Memory, error) {
	return getMemory(systats, unit)
}

func (systats *SyStats) GetSwap(unit string) (Swap, error) {
	return getSwap(systats, unit)
}

func (systats *SyStats) GetCPU() (CPU, error) {
	return getCPU(systats, 300)
}

func (systats *SyStats) GetSystem() (System, error) {
	return getSystem(systats)
}

func (systats *SyStats) GetNetworks() ([]Network, error) {
	return getNetworks()
}

func (systats *SyStats) GetNetworkUsage(networkInterface string) NetworkUsage {
	return getNetworkUsage(networkInterface)
}

func (systats *SyStats) IsServiceRunning(service string) bool {
	return isServiceRunning(service)
}

func (systats *SyStats) GetTopProcesses(count int, sort string) ([]Process, error) {
	return getTopProcesses(systats, count, sort)
}

func (systats *SyStats) GetDisks() ([]Disk, error) {
	return getDisks(systats)
}

func (systats *SyStats) IsPortOpen(port int) bool {
	return isPortOpen(port)
}

func (systats *SyStats) CanConnectExternal(url string) (bool, error) {
	return canConnect(url)
}

func (systats *SyStats) EstablishedTCPConnCount(procName string) int {
	return establishedTCPConnCount(procName)
}
