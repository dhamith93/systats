package systats

import (
	"context"
	"testing"
)

func fakeCandidates() []procCandidate {
	return []procCandidate{
		{pid: 1, cpuUsage: 5.0, memUsage: 40.0},
		{pid: 2, cpuUsage: 90.0, memUsage: 2.0},
		{pid: 3, cpuUsage: 33.0, memUsage: 10.0},
		{pid: 4, cpuUsage: 1.0, memUsage: 75.0},
		{pid: 5, cpuUsage: 60.0, memUsage: 20.0},
	}
}

func TestSortAndLimitByCPU(t *testing.T) {
	got := sortAndLimit(fakeCandidates(), "cpu", 10)

	wantOrder := []int{2, 5, 3, 1, 4}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d candidates, want %d", len(got), len(wantOrder))
	}
	for i, c := range got {
		if c.pid != wantOrder[i] {
			t.Errorf("position %d: got pid %d, want pid %d (got order: %v)", i, c.pid, wantOrder[i], pids(got))
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].cpuUsage < got[i].cpuUsage {
			t.Errorf("not sorted descending by cpuUsage at position %d: %v", i, pids(got))
		}
	}
}

func TestSortAndLimitByMemory(t *testing.T) {
	got := sortAndLimit(fakeCandidates(), "memory", 10)

	wantOrder := []int{4, 1, 5, 3, 2}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d candidates, want %d", len(got), len(wantOrder))
	}
	for i, c := range got {
		if c.pid != wantOrder[i] {
			t.Errorf("position %d: got pid %d, want pid %d (got order: %v)", i, c.pid, wantOrder[i], pids(got))
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].memUsage < got[i].memUsage {
			t.Errorf("not sorted descending by memUsage at position %d: %v", i, pids(got))
		}
	}
}

func TestSortAndLimitTruncatesToCount(t *testing.T) {
	got := sortAndLimit(fakeCandidates(), "cpu", 2)

	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2", len(got))
	}
	if got[0].pid != 2 || got[1].pid != 5 {
		t.Errorf("got top-2 by CPU %v, want pids [2 5]", pids(got))
	}
}

func TestSortAndLimitCountLargerThanInput(t *testing.T) {
	got := sortAndLimit(fakeCandidates(), "cpu", 100)
	if len(got) != 5 {
		t.Errorf("got %d candidates, want all 5 when count exceeds input length", len(got))
	}
}

func pids(candidates []procCandidate) []int {
	out := make([]int, len(candidates))
	for i, c := range candidates {
		out[i] = c.pid
	}
	return out
}

func TestInstantCPUPercent(t *testing.T) {
	cases := []struct {
		name           string
		ticks1, ticks2 uint64
		elapsedSeconds float64
		want           float64
	}{
		{"30% over 1s", 1000, 1030, 1.0, 30},
		{"full core over 1s", 1000, 1100, 1.0, 100},
		{"no change", 1000, 1000, 1.0, 0},
		{"zero elapsed guards against div by zero", 1000, 1030, 0, 0},
		{"negative elapsed guards too", 1000, 1030, -1, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := instantCPUPercent(c.ticks1, c.ticks2, c.elapsedSeconds)
			if got != c.want {
				t.Errorf("instantCPUPercent(%d, %d, %v) = %v, want %v", c.ticks1, c.ticks2, c.elapsedSeconds, got, c.want)
			}
		})
	}
}

func TestAverageCPUPercent(t *testing.T) {
	cases := []struct {
		name                string
		cpuTicks, starttime uint64
		uptimeSeconds       float64
		want                float64
	}{
		{"50% lifetime average, started at boot", 500, 0, 10, 50},
		{"100% lifetime average, started 5s after boot", 1000, 500, 15, 100},
		{"process barely older than now guards against div by zero", 100, 1000, 10, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := averageCPUPercent(c.cpuTicks, c.starttime, c.uptimeSeconds)
			if got != c.want {
				t.Errorf("averageCPUPercent(%d, %d, %v) = %v, want %v", c.cpuTicks, c.starttime, c.uptimeSeconds, got, c.want)
			}
		})
	}
}

func TestSystemUptimeSeconds(t *testing.T) {
	got, err := systemUptimeSeconds("./test_files/uptime.txt")
	if err != nil {
		t.Fatalf("systemUptimeSeconds returned error: %s", err.Error())
	}
	want := 12058.79
	if got != want {
		t.Errorf("systemUptimeSeconds() = %v, want %v", got, want)
	}
}

func procFixtureStats() *SyStats {
	return &SyStats{
		ProcPath:       "./test_files/proc",
		MeminfoPath:    "./test_files/meminfo.txt",
		UptimePath:     "./test_files/uptime.txt",
		ProcessCPUMode: CPUUsageAverage,
	}
}

func TestParseProcStat(t *testing.T) {
	// comm contains both spaces and parens - the case that breaks naive
	// field-splitting implementations.
	content := "1234 (my app (v2)) S 1 1234 1234 0 -1 4194304 5421 0 12 0 450 120 0 0 20 0 7 0 8814 2887680 5120\n"

	got, err := parseProcStat(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.comm != "my app (v2)" {
		t.Errorf("comm = %q, want %q", got.comm, "my app (v2)")
	}
	if got.state != "S" {
		t.Errorf("state = %q, want S", got.state)
	}
	if got.ppid != 1 {
		t.Errorf("ppid = %d, want 1", got.ppid)
	}
	if got.utime != 450 || got.stime != 120 {
		t.Errorf("utime/stime = %d/%d, want 450/120", got.utime, got.stime)
	}
	if got.numThreads != 7 {
		t.Errorf("numThreads = %d, want 7", got.numThreads)
	}
	if got.starttime != 8814 {
		t.Errorf("starttime = %d, want 8814", got.starttime)
	}
}

func TestParseProcStatTruncated(t *testing.T) {
	if _, err := parseProcStat("1234 (short) S 1 2 3\n"); err == nil {
		t.Errorf("expected an error for a truncated stat line")
	}
	if _, err := parseProcStat("no parens here"); err == nil {
		t.Errorf("expected an error when comm's closing paren is missing")
	}
}

func TestParseProcIO(t *testing.T) {
	content := "rchar: 892134\nwchar: 45012\nsyscr: 1204\nsyscw: 302\nread_bytes: 421888\nwrite_bytes: 12288\ncancelled_write_bytes: 4096\n"
	got := parseProcIO(content)

	if !got.Accessible {
		t.Errorf("Accessible = false, want true")
	}
	if got.ReadChars != 892134 || got.WriteChars != 45012 {
		t.Errorf("chars = %d/%d, want 892134/45012", got.ReadChars, got.WriteChars)
	}
	if got.ReadBytes != 421888 || got.WriteBytes != 12288 {
		t.Errorf("bytes = %d/%d, want 421888/12288", got.ReadBytes, got.WriteBytes)
	}
	if got.ReadSyscalls != 1204 || got.WriteSyscalls != 302 {
		t.Errorf("syscalls = %d/%d, want 1204/302", got.ReadSyscalls, got.WriteSyscalls)
	}
	if got.CancelledWriteBytes != 4096 {
		t.Errorf("CancelledWriteBytes = %d, want 4096", got.CancelledWriteBytes)
	}
}

func TestParseProcIOSkipsBadLines(t *testing.T) {
	// A garbage value must not panic or poison the other counters.
	got := parseProcIO("rchar: 100\nwchar: notanumber\nread_bytes: 200\n")
	if got.ReadChars != 100 || got.ReadBytes != 200 {
		t.Errorf("good lines should still parse, got %+v", got)
	}
	if got.WriteChars != 0 {
		t.Errorf("WriteChars = %d, want 0 for an unparseable value", got.WriteChars)
	}
}

func TestProcessStateName(t *testing.T) {
	cases := map[string]string{
		"R": "running", "S": "sleeping", "D": "disk-sleep", "Z": "zombie",
		"T": "stopped", "t": "tracing-stop", "X": "dead", "x": "dead",
		"K": "wakekill", "W": "waking", "P": "parked", "I": "idle",
		"Q": "unknown", "": "unknown",
	}
	for state, want := range cases {
		if got := processStateName(state); got != want {
			t.Errorf("processStateName(%q) = %q, want %q", state, got, want)
		}
	}
}

func TestParseCmdline(t *testing.T) {
	got := parseCmdline("/usr/bin/foo\x00--flag\x00bar\x00")
	if got != "/usr/bin/foo --flag bar" {
		t.Errorf("got %q, want %q", got, "/usr/bin/foo --flag bar")
	}
	if parseCmdline("") != "" {
		t.Errorf("empty cmdline should stay empty")
	}
}

func TestListPidsFromFixtureTree(t *testing.T) {
	pids, err := listPids("./test_files/proc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pids) != 2 {
		t.Fatalf("got %v, want [1234 5678] - non-numeric entries must be skipped", pids)
	}
}

// TestGetProcessAverageMode is a full end-to-end GetProcess against the
// fixture tree. Average mode has no sleep and no wall-clock dependency,
// so every value here is deterministic.
func TestGetProcessAverageMode(t *testing.T) {
	got, err := getProcess(context.Background(), procFixtureStats(), 1234)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Pid != 1234 {
		t.Errorf("Pid = %d, want 1234", got.Pid)
	}
	if got.Name != "my app (v2)" {
		t.Errorf("Name = %q, want %q", got.Name, "my app (v2)")
	}
	if got.ExecPath != "/usr/bin/myapp --config /etc/myapp.conf" {
		t.Errorf("ExecPath = %q", got.ExecPath)
	}
	if got.State != "S" || got.StateName != "sleeping" {
		t.Errorf("state = %q/%q, want S/sleeping", got.State, got.StateName)
	}
	if got.Threads != 7 {
		t.Errorf("Threads = %d, want 7", got.Threads)
	}
	if got.OpenFDs != 4 || !got.FDsAccessible {
		t.Errorf("OpenFDs = %d (accessible=%v), want 4 true", got.OpenFDs, got.FDsAccessible)
	}
	if !got.IO.Accessible || got.IO.ReadBytes != 421888 {
		t.Errorf("IO = %+v, want accessible with ReadBytes 421888", got.IO)
	}

	// Computed with the same helpers rather than hand-arithmetic.
	wantCPU := float32(averageCPUPercent(450+120, 8814, 12058.79))
	if got.CPUUsage != wantCPU {
		t.Errorf("CPUUsage = %v, want %v", got.CPUUsage, wantCPU)
	}
	wantMem := float32(100 * float64(20480) / float64(16315340))
	if got.MemUsage != wantMem {
		t.Errorf("MemUsage = %v, want %v", got.MemUsage, wantMem)
	}
}

// TestGetProcessDegradesWhenIOAndFDsUnreadable covers a kernel-thread-like
// process: no io file, no fd directory. Neither is fatal.
func TestGetProcessDegradesWhenIOAndFDsUnreadable(t *testing.T) {
	got, err := getProcess(context.Background(), procFixtureStats(), 5678)
	if err != nil {
		t.Fatalf("missing io/fd must not fail the lookup, got %v", err)
	}

	if got.IO.Accessible {
		t.Errorf("IO.Accessible = true, want false when /proc/<pid>/io is absent")
	}
	if got.FDsAccessible {
		t.Errorf("FDsAccessible = true, want false when /proc/<pid>/fd is absent")
	}
	// Everything else still populated.
	if got.Name != "kworker/0:1" {
		t.Errorf("Name = %q, want kworker/0:1", got.Name)
	}
	if got.StateName != "zombie" {
		t.Errorf("StateName = %q, want zombie", got.StateName)
	}
	if got.Threads != 1 {
		t.Errorf("Threads = %d, want 1", got.Threads)
	}
}

func TestGetProcessUnknownPid(t *testing.T) {
	if _, err := getProcess(context.Background(), procFixtureStats(), 99999); err == nil {
		t.Errorf("expected an error for a pid that doesn't exist")
	}
}

func TestValidateSortBy(t *testing.T) {
	for _, ok := range []string{"", SortByCPU, SortByMemory} {
		if err := validateSortBy(ok); err != nil {
			t.Errorf("validateSortBy(%q) = %v, want nil", ok, err)
		}
	}
	// A typo used to silently sort by CPU and report success.
	for _, bad := range []string{"memroy", "CPU", "ram", "cpu "} {
		if err := validateSortBy(bad); err == nil {
			t.Errorf("validateSortBy(%q) = nil, want an error", bad)
		}
	}
}

func TestGetTopProcessesRejectsUnknownSort(t *testing.T) {
	syStats := procFixtureStats()
	if _, err := getTopProcesses(context.Background(), syStats, 5, "memroy"); err == nil {
		t.Errorf("expected an error for a misspelled sort order")
	}
}

func TestSortAndLimitUsesConstants(t *testing.T) {
	// SortByMemory must select the memory ordering; anything else is CPU.
	byMem := sortAndLimit(fakeCandidates(), SortByMemory, 1)
	if byMem[0].pid != 4 {
		t.Errorf("SortByMemory picked pid %d, want 4", byMem[0].pid)
	}
	byCPU := sortAndLimit(fakeCandidates(), SortByCPU, 1)
	if byCPU[0].pid != 2 {
		t.Errorf("SortByCPU picked pid %d, want 2", byCPU[0].pid)
	}
}
