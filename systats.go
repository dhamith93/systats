package systats

const (
	Kilobyte string = "KB"
	Megabyte string = "MB"
	Gigabyte string = "GB"
)

type SyStats struct {
	MeminfoPath string
}

func New() SyStats {
	return SyStats{
		MeminfoPath: "/proc/meminfo",
	}
}

func GetMemory(systats SyStats, unit string) (Memory, error) {
	return getMemory(&systats, unit)
}
