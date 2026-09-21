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
	Pid      int     `json:"pid"`
	ExecPath string  `json:"execPath"`
	User     string  `json:"user"`
	CPUUsage float32 `json:"cpuUsage"`
	MemUsage float32 `json:"memUsage"`
}

const (
	clockTicksPerSec = 100
	cpuSampleWindow  = 300 * time.Millisecond
)

// procStat holds the CPU-accounting fields read from /proc/<pid>/stat:
// utime/stime (cumulative CPU ticks) and starttime (ticks since boot when
// the process started, used only in CPUUsageAverage mode).
type procStat struct {
	utime     uint64
	stime     uint64
	starttime uint64
}

// procCandidate carries just enough to rank a process (CPU/mem usage) plus
// its raw uid. cmdline and username resolution are deferred until after
// sorting, since those are only ever needed for the processes that
// actually make the final top-count cut.
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

	var candidates []procCandidate
	if systats.ProcessCPUMode == CPUUsageAverage {
		candidates, err = collectCandidatesAverage(systats.UptimePath, pids, totalMemKB)
	} else {
		candidates, err = collectCandidatesInstant(pids, totalMemKB)
	}
	if err != nil {
		return nil, err
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

// collectCandidatesInstant computes CPU usage as an instantaneous value:
// sample every pid, sleep cpuSampleWindow, sample again, and use the
// delta over actual elapsed wall time. Matches `top`'s default behavior;
// costs at least cpuSampleWindow in latency.
func collectCandidatesInstant(pids []int, totalMemKB uint64) ([]procCandidate, error) {
	start := time.Now()
	sample1 := sampleProcStats(pids)
	time.Sleep(cpuSampleWindow)
	sample2 := sampleProcStats(pids)
	elapsedSeconds := time.Since(start).Seconds()

	candidates := []procCandidate{}
	for _, pid := range pids {
		s2, ok := sample2[pid]
		if !ok {
			continue // process exited during sampling
		}
		s1, ok := sample1[pid]
		if !ok {
			continue
		}

		uid, vmRSSKB, err := readProcMemAndUID(pid)
		if err != nil {
			continue // process exited
		}

		cpuUsage := instantCPUPercent(s1.utime+s1.stime, s2.utime+s2.stime, elapsedSeconds)
		memUsage := 100 * float64(vmRSSKB) / float64(totalMemKB)

		candidates = append(candidates, procCandidate{
			pid:      pid,
			uid:      uid,
			cpuUsage: cpuUsage,
			memUsage: memUsage,
		})
	}
	return candidates, nil
}

// collectCandidatesAverage computes CPU usage as a lifetime average since
// each process started (total CPU time / time since start), matching
// `ps`'s default %cpu. Single pass, no sampling wait.
func collectCandidatesAverage(uptimePath string, pids []int, totalMemKB uint64) ([]procCandidate, error) {
	uptimeSeconds, err := systemUptimeSeconds(uptimePath)
	if err != nil {
		return nil, err
	}

	stats := sampleProcStats(pids)

	candidates := []procCandidate{}
	for _, pid := range pids {
		s, ok := stats[pid]
		if !ok {
			continue // process exited
		}

		uid, vmRSSKB, err := readProcMemAndUID(pid)
		if err != nil {
			continue // process exited
		}

		cpuUsage := averageCPUPercent(s.utime+s.stime, s.starttime, uptimeSeconds)
		memUsage := 100 * float64(vmRSSKB) / float64(totalMemKB)

		candidates = append(candidates, procCandidate{
			pid:      pid,
			uid:      uid,
			cpuUsage: cpuUsage,
			memUsage: memUsage,
		})
	}
	return candidates, nil
}

// instantCPUPercent computes CPU usage from two /proc/<pid>/stat samples
// (each utime+stime) taken elapsedSeconds apart.
func instantCPUPercent(cpuTicks1, cpuTicks2 uint64, elapsedSeconds float64) float64 {
	if elapsedSeconds <= 0 {
		return 0
	}
	deltaTicks := float64(cpuTicks2 - cpuTicks1)
	return 100 * (deltaTicks / clockTicksPerSec) / elapsedSeconds
}

// averageCPUPercent computes CPU usage as a lifetime average: total CPU
// time (cpuTicks = utime+stime) divided by time since the process
// started (uptimeSeconds - starttime, both relative to boot).
func averageCPUPercent(cpuTicks, starttime uint64, uptimeSeconds float64) float64 {
	processAgeSeconds := uptimeSeconds - float64(starttime)/clockTicksPerSec
	if processAgeSeconds <= 0 {
		return 0
	}
	return 100 * (float64(cpuTicks) / clockTicksPerSec) / processAgeSeconds
}

// sortAndLimit sorts candidates descending by CPU or memory usage and
// truncates to count. Kept separate from getTopProcesses so it can be
// unit-tested with synthetic data, without needing /proc.
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

// sampleProcStats reads utime/stime/starttime for each pid. Entries for
// processes that can no longer be read (exited, permission denied) are
// simply omitted rather than failing the whole batch.
func sampleProcStats(pids []int) map[int]procStat {
	out := make(map[int]procStat, len(pids))
	for _, pid := range pids {
		s, err := readProcStat(pid)
		if err != nil {
			continue
		}
		out[pid] = s
	}
	return out
}

// readProcStat parses /proc/<pid>/stat. comm (field 2) is parenthesized
// and may itself contain spaces or parens, so this locates the last ')'
// and parses everything after it positionally rather than splitting the
// whole line on spaces.
func readProcStat(pid int) (procStat, error) {
	content, err := fileops.ReadFileWithError("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return procStat{}, err
	}

	idx := strings.LastIndex(content, ")")
	if idx == -1 || idx+2 > len(content) {
		return procStat{}, errors.New("unexpected /proc/<pid>/stat format")
	}
	fields := strings.Fields(content[idx+2:])
	if len(fields) < 20 {
		return procStat{}, errors.New("unexpected /proc/<pid>/stat format")
	}

	return procStat{
		utime:     strops.ToUint64(fields[11]), // field 14: utime
		stime:     strops.ToUint64(fields[12]), // field 15: stime
		starttime: strops.ToUint64(fields[19]), // field 22: starttime
	}, nil
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

func systemUptimeSeconds(uptimePath string) (float64, error) {
	content, err := fileops.ReadFileWithError(uptimePath)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(content)
	if len(fields) < 1 {
		return 0, errors.New("unexpected " + uptimePath + " format")
	}
	return strops.ToFloat64(fields[0]), nil
}
