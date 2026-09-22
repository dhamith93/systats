package systats

import (
	"errors"
	"os"
	"os/user"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"github.com/dhamith93/systats/internal/strops"
)

// Process holds information on single process
type Process struct {
	Pid int `json:"pid"`
	// Name is the kernel's short process name (stat's comm field). It's
	// the only name a kernel thread has, since those have no cmdline.
	Name     string `json:"name"`
	ExecPath string `json:"execPath"`
	User     string `json:"user"`
	// State is the raw single-letter state from /proc/<pid>/stat ("R",
	// "S", "Z", ...); StateName is its readable form.
	State     string  `json:"state"`
	StateName string  `json:"stateName"`
	Threads   int     `json:"threads"`
	CPUUsage  float32 `json:"cpuUsage"`
	MemUsage  float32 `json:"memUsage"`
	// OpenFDs is meaningful only when FDsAccessible - reading another
	// user's /proc/<pid>/fd requires matching ownership or root.
	OpenFDs       int       `json:"openFds"`
	FDsAccessible bool      `json:"fdsAccessible"`
	IO            ProcessIO `json:"io"`
}

// ProcessIO holds per-process I/O counters from /proc/<pid>/io.
//
// ReadChars/WriteChars count bytes passed to read/write syscalls,
// including page-cache hits; ReadBytes/WriteBytes count bytes that
// actually reached the block layer. They routinely differ by orders of
// magnitude - a process re-reading a cached file racks up ReadChars with
// no ReadBytes at all.
type ProcessIO struct {
	ReadChars           uint64 `json:"readChars"`
	WriteChars          uint64 `json:"writeChars"`
	ReadSyscalls        uint64 `json:"readSyscalls"`
	WriteSyscalls       uint64 `json:"writeSyscalls"`
	ReadBytes           uint64 `json:"readBytes"`
	WriteBytes          uint64 `json:"writeBytes"`
	CancelledWriteBytes uint64 `json:"cancelledWriteBytes"`
	// Accessible is false when /proc/<pid>/io couldn't be read - it's
	// owner-or-root only, and absent entirely for kernel threads. Every
	// counter is zero in that case, so check this before reading the
	// zeros as "no I/O".
	Accessible bool `json:"accessible"`
}

const (
	clockTicksPerSec = 100
	cpuSampleWindow  = 300 * time.Millisecond
)

// procStat holds the fields read from /proc/<pid>/stat: comm/state/ppid,
// utime/stime (cumulative CPU ticks), num_threads, and starttime (ticks
// since boot when the process started, used in CPUUsageAverage mode).
type procStat struct {
	comm       string
	state      string
	ppid       int
	utime      uint64
	stime      uint64
	numThreads int
	starttime  uint64
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
	pids, err := listPids(systats.ProcPath)
	if err != nil {
		return nil, err
	}

	totalMemKB, err := totalMemoryKB(systats.MeminfoPath)
	if err != nil {
		return nil, err
	}

	var candidates []procCandidate
	if systats.ProcessCPUMode == CPUUsageAverage {
		candidates, err = collectCandidatesAverage(systats, pids, totalMemKB)
	} else {
		candidates, err = collectCandidatesInstant(systats, pids, totalMemKB)
	}
	if err != nil {
		return nil, err
	}

	candidates = sortAndLimit(candidates, sortBy, count)

	// Only the processes that survived sorting pay for the more expensive
	// per-process reads (cmdline, stat detail, fd count, io) and the
	// uid->username resolution.
	out := make([]Process, 0, len(candidates))
	for _, c := range candidates {
		p := Process{
			Pid:      c.pid,
			User:     resolveUsername(c.uid),
			CPUUsage: float32(c.cpuUsage),
			MemUsage: float32(c.memUsage),
		}
		if err := enrichProcess(systats, &p); err != nil {
			continue // process exited after sampling
		}
		out = append(out, p)
	}

	return out, nil
}

// getProcess looks up a single process by pid. Unlike getTopProcesses,
// which skips processes it can't read, this returns an error when
// /proc/<pid>/stat is unreadable - a targeted lookup of a pid that isn't
// there should say so rather than return an empty result.
func getProcess(systats *SyStats, pid int) (Process, error) {
	totalMemKB, err := totalMemoryKB(systats.MeminfoPath)
	if err != nil {
		return Process{}, err
	}

	var cpuUsage float64
	if systats.ProcessCPUMode == CPUUsageAverage {
		uptimeSeconds, err := systemUptimeSeconds(systats.UptimePath)
		if err != nil {
			return Process{}, err
		}
		s, err := readProcStat(systats.ProcPath, pid)
		if err != nil {
			return Process{}, err
		}
		cpuUsage = averageCPUPercent(s.utime+s.stime, s.starttime, uptimeSeconds)
	} else {
		// Same two-sample window GetTopProcesses uses, so this call also
		// costs at least cpuSampleWindow.
		start := time.Now()
		s1, err := readProcStat(systats.ProcPath, pid)
		if err != nil {
			return Process{}, err
		}
		time.Sleep(cpuSampleWindow)
		s2, err := readProcStat(systats.ProcPath, pid)
		if err != nil {
			return Process{}, err
		}
		cpuUsage = instantCPUPercent(s1.utime+s1.stime, s2.utime+s2.stime, time.Since(start).Seconds())
	}

	uid, vmRSSKB, err := readProcMemAndUID(systats.ProcPath, pid)
	if err != nil {
		return Process{}, err
	}

	p := Process{
		Pid:      pid,
		User:     resolveUsername(uid),
		CPUUsage: float32(cpuUsage),
		MemUsage: float32(100 * float64(vmRSSKB) / float64(totalMemKB)),
	}
	if err := enrichProcess(systats, &p); err != nil {
		return Process{}, err
	}

	return p, nil
}

// enrichProcess fills in the descriptive fields shared by getProcess and
// getTopProcesses. Only an unreadable stat file is fatal: cmdline is
// legitimately empty for kernel threads, and fd/io are permission-gated,
// so those degrade to zero values with their Accessible flags unset.
func enrichProcess(systats *SyStats, p *Process) error {
	s, err := readProcStat(systats.ProcPath, p.Pid)
	if err != nil {
		return err
	}
	p.Name = s.comm
	p.State = s.state
	p.StateName = processStateName(s.state)
	p.Threads = s.numThreads

	if content, err := fileops.ReadFileWithError(procFilePath(systats.ProcPath, p.Pid, "cmdline")); err == nil {
		p.ExecPath = parseCmdline(content)
	}

	p.OpenFDs, p.FDsAccessible = countOpenFDs(systats.ProcPath, p.Pid)
	p.IO = readProcIO(systats.ProcPath, p.Pid)

	return nil
}

// procFilePath builds /proc/<pid>/<name> against the configured proc
// root, so tests can point at a fixture tree.
func procFilePath(procPath string, pid int, name string) string {
	return path.Join(procPath, strconv.Itoa(pid), name)
}

// processStateName maps /proc/<pid>/stat's single-letter state to a
// readable name (see the process state table in proc(5)).
func processStateName(state string) string {
	switch state {
	case "R":
		return "running"
	case "S":
		return "sleeping"
	case "D":
		return "disk-sleep"
	case "Z":
		return "zombie"
	case "T":
		return "stopped"
	case "t":
		return "tracing-stop"
	case "X", "x":
		return "dead"
	case "K":
		return "wakekill"
	case "W":
		return "waking"
	case "P":
		return "parked"
	case "I":
		return "idle"
	default:
		return "unknown"
	}
}

// parseCmdline turns /proc/<pid>/cmdline's NUL-separated argv into a
// readable string.
func parseCmdline(content string) string {
	return strings.TrimSpace(strings.ReplaceAll(content, "\x00", " "))
}

// countOpenFDs counts entries in /proc/<pid>/fd. ok is false when the
// directory can't be read, which for another user's process is the norm
// rather than an error.
func countOpenFDs(procPath string, pid int) (count int, ok bool) {
	entries, err := os.ReadDir(path.Join(procPath, strconv.Itoa(pid), "fd"))
	if err != nil {
		return 0, false
	}
	return len(entries), true
}

func readProcIO(procPath string, pid int) ProcessIO {
	content, err := fileops.ReadFileWithError(procFilePath(procPath, pid, "io"))
	if err != nil {
		return ProcessIO{}
	}
	return parseProcIO(content)
}

// parseProcIO parses /proc/<pid>/io's "key: value" lines. Unparseable
// values are skipped rather than panicking: this file may be partially
// readable, and it's supplementary data on top of an already-valid
// process reading.
func parseProcIO(content string) ProcessIO {
	io := ProcessIO{Accessible: true}

	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "rchar":
			io.ReadChars = value
		case "wchar":
			io.WriteChars = value
		case "syscr":
			io.ReadSyscalls = value
		case "syscw":
			io.WriteSyscalls = value
		case "read_bytes":
			io.ReadBytes = value
		case "write_bytes":
			io.WriteBytes = value
		case "cancelled_write_bytes":
			io.CancelledWriteBytes = value
		}
	}

	return io
}

// collectCandidatesInstant computes CPU usage as an instantaneous value:
// sample every pid, sleep cpuSampleWindow, sample again, and use the
// delta over actual elapsed wall time. Matches `top`'s default behavior;
// costs at least cpuSampleWindow in latency.
func collectCandidatesInstant(systats *SyStats, pids []int, totalMemKB uint64) ([]procCandidate, error) {
	start := time.Now()
	sample1 := sampleProcStats(systats.ProcPath, pids)
	time.Sleep(cpuSampleWindow)
	sample2 := sampleProcStats(systats.ProcPath, pids)
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

		uid, vmRSSKB, err := readProcMemAndUID(systats.ProcPath, pid)
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
func collectCandidatesAverage(systats *SyStats, pids []int, totalMemKB uint64) ([]procCandidate, error) {
	uptimeSeconds, err := systemUptimeSeconds(systats.UptimePath)
	if err != nil {
		return nil, err
	}

	stats := sampleProcStats(systats.ProcPath, pids)

	candidates := []procCandidate{}
	for _, pid := range pids {
		s, ok := stats[pid]
		if !ok {
			continue // process exited
		}

		uid, vmRSSKB, err := readProcMemAndUID(systats.ProcPath, pid)
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

func listPids(procPath string) ([]int, error) {
	entries, err := os.ReadDir(procPath)
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

// sampleProcStats reads the stat fields for each pid. Entries for
// processes that can no longer be read (exited, permission denied) are
// simply omitted rather than failing the whole batch.
func sampleProcStats(procPath string, pids []int) map[int]procStat {
	out := make(map[int]procStat, len(pids))
	for _, pid := range pids {
		s, err := readProcStat(procPath, pid)
		if err != nil {
			continue
		}
		out[pid] = s
	}
	return out
}

func readProcStat(procPath string, pid int) (procStat, error) {
	content, err := fileops.ReadFileWithError(procFilePath(procPath, pid, "stat"))
	if err != nil {
		return procStat{}, err
	}
	return parseProcStat(content)
}

// parseProcStat parses /proc/<pid>/stat. comm (field 2) is parenthesized
// and may itself contain spaces or parens, so this locates the last ')'
// and parses everything after it positionally rather than splitting the
// whole line on spaces. Positions after comm are 1-based stat field N at
// fields[N-3].
func parseProcStat(content string) (procStat, error) {
	idx := strings.LastIndex(content, ")")
	if idx == -1 || idx+2 > len(content) {
		return procStat{}, errors.New("unexpected /proc/<pid>/stat format")
	}
	fields := strings.Fields(content[idx+2:])
	if len(fields) < 20 {
		return procStat{}, errors.New("unexpected /proc/<pid>/stat format")
	}

	s := procStat{
		state:     fields[0],                   // field 3: state
		utime:     strops.ToUint64(fields[11]), // field 14: utime
		stime:     strops.ToUint64(fields[12]), // field 15: stime
		starttime: strops.ToUint64(fields[19]), // field 22: starttime
	}
	s.ppid, _ = strconv.Atoi(fields[1])        // field 4: ppid
	s.numThreads, _ = strconv.Atoi(fields[17]) // field 20: num_threads

	// comm sits between the first '(' and that last ')'.
	if open := strings.Index(content, "("); open != -1 && open < idx {
		s.comm = content[open+1 : idx]
	}

	return s, nil
}

func readProcMemAndUID(procPath string, pid int) (uid string, vmRSSKB uint64, err error) {
	content, err := fileops.ReadFileWithError(procFilePath(procPath, pid, "status"))
	if err != nil {
		return "", 0, err
	}
	uid, vmRSSKB = parseProcStatus(content)
	return uid, vmRSSKB, nil
}

func parseProcStatus(content string) (uid string, vmRSSKB uint64) {
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
	return uid, vmRSSKB
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
