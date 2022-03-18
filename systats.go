package systats

const (
	Byte     string = "B"
	Kilobyte string = "KB"
	Megabyte string = "MB"
	Gigabyte string = "GB"
)

// SyStats holds information used to collect data
type SyStats struct {
	MeminfoPath     string
	StatFilePath    string
	CPUinfoFilePath string
	VersionPath     string
	EtcPath         string
	UptimePath      string
}

func New() SyStats {
	return SyStats{
		MeminfoPath:     "/proc/meminfo",
		StatFilePath:    "/proc/stat",
		CPUinfoFilePath: "/proc/cpuinfo",
		VersionPath:     "/proc/version",
		EtcPath:         "/etc/",
		UptimePath:      "/proc/uptime",
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
	if sort == "cpu" {
		sort = "-pcpu"
	}
	if sort == "memory" {
		sort = "-pmem"
	}
	return getTopProcesses(count, sort)
}

func (systats *SyStats) GetDisks() ([]Disk, error) {
	return getDisks()
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
