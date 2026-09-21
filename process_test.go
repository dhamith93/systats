package systats

import "testing"

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
