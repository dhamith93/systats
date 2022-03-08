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
}

func New() SyStats {
	return SyStats{
		MeminfoPath:     "/proc/meminfo",
		StatFilePath:    "/proc/stat",
		CPUinfoFilePath: "/proc/cpuinfo",
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
