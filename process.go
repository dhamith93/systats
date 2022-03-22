package systats

import (
	"strconv"
	"strings"

	"github.com/dhamith93/systats/exec"
	"github.com/dhamith93/systats/internal/fileops"
)

// Process holds information on single process
type Process struct {
	Pid      int
	ExecPath string
	User     string
	CPUUsage float32
	MemUsage float32
}

func getTopProcesses(count int, sort string) ([]Process, error) {
	result := exec.Execute("ps", "-eo", "pid,%cpu,%mem,user", "--no-headers", "--sort="+sort)
	resultArray := strings.Split(result, "\n")
	out := []Process{}

	for i, process := range resultArray {
		if i+1 > count {
			break
		}

		processArray := strings.Fields(process)
		if len(processArray) == 0 {
			continue
		}
		pid, err := strconv.Atoi(processArray[0])
		if err != nil {
			return out, err
		}
		cpuUsage, err := strconv.ParseFloat(processArray[1], 32)
		if err != nil {
			return out, err
		}
		memUsage, err := strconv.ParseFloat(processArray[2], 32)
		if err != nil {
			return out, err
		}
		execPath, err := fileops.ReadFileWithError("/proc/" + processArray[0] + "/cmdline")
		if err != nil {
			continue
		}

		out = append(out, Process{
			Pid:      pid,
			CPUUsage: float32(cpuUsage),
			MemUsage: float32(memUsage),
			User:     processArray[3],
			ExecPath: execPath,
		})
	}

	return out, nil
}
