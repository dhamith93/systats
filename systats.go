package systats

const (
	Kilobyte string = "KB"
	Megabyte string = "MB"
	Gigabyte string = "GB"
)

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

func GetMemory(systats SyStats, unit string) (Memory, error) {
	return getMemory(&systats, unit)
}

func GetSwap(systats SyStats, unit string) (Swap, error) {
	return getSwap(&systats, unit)
}

func GetCPU(systats SyStats) (CPU, error) {
	return getCPU(&systats, 300)
}

func GetSystem(systats SyStats) (System, error) {
	return getSystem(&systats)
}

func GetNetworks(systats SyStats) ([]Network, error) {
	return getNetworks(&systats)
}

func IsServiceRunning(service string) bool {
	return isServiceRunning(service)
}

func GetTopProcesses(count int, sort string) ([]Process, error) {
	if sort == "cpu" {
		sort = "-pcpu"
	}
	if sort == "memory" {
		sort = "-pmem"
	}
	return getTopProcesses(count, sort)
}
