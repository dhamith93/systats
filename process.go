package systats

import (
	"errors"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"github.com/dhamith93/systats/internal/strops"
)

// Process holds information on single process
type Process struct {
	Pid      int
	ExecPath string
	User     string
	CPUUsage float32
	MemUsage float32
}

const (
	clockTicksPerSec = 100
	cpuSampleWindow  = 300 * time.Millisecond
)

type procTimes struct {
	utime uint64
	stime uint64
}

type procCandidate struct {
	pid      int
	uid      string
	cpuUsage float64
	memUsage float64
}

func getTopProcesses(systats *SyStats, count int, sortBy string) ([]Process, error) {
	pids, err := listPids()
	if err != nil {
		return nil, err
	}

	totalMemKB, err := totalMemoryKB(systats.MeminfoPath)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	sample1 := sampleProcTimes(pids)
	time.Sleep(cpuSampleWindow)
	sample2 := sampleProcTimes(pids)
	elapsedSeconds := time.Since(start).Seconds()

	candidates := []procCandidate{}
	for _, pid := range pids {
		t2, ok := sample2[pid]
		if !ok {
			continue // process exited during sampling
		}
		t1, ok := sample1[pid]
		if !ok {
			continue
		}

		uid, vmRSSKB, err := readProcMemAndUID(pid)
		if err != nil {
			continue // process exited
		}

		deltaTicks := float64((t2.utime + t2.stime) - (t1.utime + t1.stime))
		cpuUsage := 0.0
		if elapsedSeconds > 0 {
			cpuUsage = 100 * (deltaTicks / clockTicksPerSec) / elapsedSeconds
		}
		memUsage := 100 * float64(vmRSSKB) / float64(totalMemKB)

		candidates = append(candidates, procCandidate{
			pid:      pid,
			uid:      uid,
			cpuUsage: cpuUsage,
			memUsage: memUsage,
		})
	}

	candidates = sortAndLimit(candidates, sortBy, count)

	// Only the processes that survived sorting pay for the more expensive
	// cmdline read and uid->username resolution.
	out := make([]Process, 0, len(candidates))
	for _, c := range candidates {
		execPath, err := fileops.ReadFileWithError("/proc/" + strconv.Itoa(c.pid) + "/cmdline")
		if err != nil {
			continue // process exited after sampling
		}
		execPath = strings.TrimSpace(strings.ReplaceAll(execPath, "\x00", " "))

		out = append(out, Process{
			Pid:      c.pid,
			ExecPath: execPath,
			User:     resolveUsername(c.uid),
			CPUUsage: float32(c.cpuUsage),
			MemUsage: float32(c.memUsage),
		})
	}

	return out, nil
}

func sortAndLimit(candidates []procCandidate, sortBy string, count int) []procCandidate {
	if sortBy == "memory" {
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].memUsage > candidates[j].memUsage })
	} else {
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].cpuUsage > candidates[j].cpuUsage })
	}

	if len(candidates) > count {
		candidates = candidates[:count]
	}

	return candidates
}

func listPids() ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	pids := []int{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

func sampleProcTimes(pids []int) map[int]procTimes {
	out := make(map[int]procTimes, len(pids))
	for _, pid := range pids {
		content, err := fileops.ReadFileWithError("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			continue
		}

		// comm (field 2) is parenthesized and may itself contain spaces
		// or parens, so locate the last ')' and parse everything after
		// it positionally rather than splitting the whole line on spaces.
		idx := strings.LastIndex(content, ")")
		if idx == -1 || idx+2 > len(content) {
			continue
		}
		fields := strings.Fields(content[idx+2:])
		if len(fields) < 13 {
			continue
		}

		out[pid] = procTimes{
			utime: strops.ToUint64(fields[11]), // field 14: utime
			stime: strops.ToUint64(fields[12]), // field 15: stime
		}
	}
	return out
}

func readProcMemAndUID(pid int) (uid string, vmRSSKB uint64, err error) {
	content, err := fileops.ReadFileWithError("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return "", 0, err
	}

	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "Uid:" {
			uid = fields[1]
		}
		if fields[0] == "VmRSS:" {
			vmRSSKB = strops.ToUint64(fields[1])
		}
	}

	return uid, vmRSSKB, nil
}

func resolveUsername(uid string) string {
	if u, err := user.LookupId(uid); err == nil {
		return u.Username
	}
	return uid
}

func totalMemoryKB(meminfoPath string) (uint64, error) {
	content, err := fileops.ReadFileWithError(meminfoPath)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			return strops.ToUint64(fields[1]), nil
		}
	}
	return 0, errors.New("MemTotal not found in " + meminfoPath)
}
