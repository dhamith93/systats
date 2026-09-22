package systats

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// cpuFixtureStats points the CPU code at the fixture tree so these tests
// run anywhere, not just on Linux. Reading the same stat fixture for both
// samples yields a zero delta, which is fine - what's under test here is
// the timing and cancellation behavior, not the arithmetic.
func cpuFixtureStats() *SyStats {
	return &SyStats{
		StatFilePath:    "./test_files/stat.txt",
		CPUinfoFilePath: "./test_files/cpuinfo.txt",
		LoadAvgPath:     "./test_files/loadavg.txt",
	}
}

func TestSleepCtxCompletes(t *testing.T) {
	if err := sleepCtx(context.Background(), 10*time.Millisecond); err != nil {
		t.Errorf("sleepCtx() on an uncancelled context = %v, want nil", err)
	}
}

func TestSleepCtxReturnsImmediatelyWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := sleepCtx(ctx, 5*time.Second)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("sleepCtx() = %v, want context.Canceled", err)
	}
	// The point of the helper: a cancelled caller does not wait out the
	// full duration. A plain time.Sleep would have taken 5s here.
	if elapsed > time.Second {
		t.Errorf("sleepCtx() took %v on a cancelled context, want well under the 5s duration", elapsed)
	}
}

func TestSleepCtxHonorsDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if err := sleepCtx(ctx, 5*time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("sleepCtx() = %v, want context.DeadlineExceeded", err)
	}
}

func TestCPUSampleWindowDefaults(t *testing.T) {
	// A hand-constructed SyStats must not end up sampling over no time at
	// all - a zero window would make every CPU percentage meaningless.
	var zero SyStats
	if got := zero.cpuSampleWindow(); got != defaultCPUSampleWindow {
		t.Errorf("zero-value SyStats.cpuSampleWindow() = %v, want %v", got, defaultCPUSampleWindow)
	}

	fromNew := New()
	if got := fromNew.cpuSampleWindow(); got != defaultCPUSampleWindow {
		t.Errorf("New().cpuSampleWindow() = %v, want %v", got, defaultCPUSampleWindow)
	}

	negative := SyStats{CPUSampleWindow: -1 * time.Second}
	if got := negative.cpuSampleWindow(); got != defaultCPUSampleWindow {
		t.Errorf("negative CPUSampleWindow = %v, want the default %v", got, defaultCPUSampleWindow)
	}

	custom := SyStats{CPUSampleWindow: 50 * time.Millisecond}
	if got := custom.cpuSampleWindow(); got != 50*time.Millisecond {
		t.Errorf("custom CPUSampleWindow = %v, want 50ms", got)
	}
}

func TestGetCPUWithContextAbortsOnCancel(t *testing.T) {
	syStats := cpuFixtureStats()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := syStats.GetCPUWithContext(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetCPUWithContext() = %v, want context.Canceled", err)
	}
	if elapsed >= defaultCPUSampleWindow {
		t.Errorf("GetCPUWithContext() took %v on a cancelled context, want less than the %v sample window", elapsed, defaultCPUSampleWindow)
	}
}

func TestGetCPUHonorsSampleWindow(t *testing.T) {
	syStats := cpuFixtureStats()
	syStats.CPUSampleWindow = 20 * time.Millisecond

	start := time.Now()
	if _, err := syStats.GetCPU(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < 20*time.Millisecond {
		t.Errorf("GetCPU() returned in %v, faster than the %v window it was told to sample over", elapsed, syStats.CPUSampleWindow)
	}
	// The whole point of the knob: materially faster than the default.
	if elapsed >= defaultCPUSampleWindow {
		t.Errorf("GetCPU() took %v with a 20ms window, want well under the %v default", elapsed, defaultCPUSampleWindow)
	}
}

func TestGetProcessWithContextAbortsOnCancel(t *testing.T) {
	syStats := procFixtureStats()
	// Average mode has no sampling window, so force the instant path that
	// actually sleeps.
	syStats.ProcessCPUMode = CPUUsageInstant

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := syStats.GetProcessWithContext(ctx, 1234)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetProcessWithContext() = %v, want context.Canceled", err)
	}
	if elapsed >= defaultCPUSampleWindow {
		t.Errorf("GetProcessWithContext() took %v on a cancelled context, want less than the %v sample window", elapsed, defaultCPUSampleWindow)
	}
}

func TestGetTopProcessesWithContextAbortsOnCancel(t *testing.T) {
	syStats := procFixtureStats()
	syStats.ProcessCPUMode = CPUUsageInstant

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := syStats.GetTopProcessesWithContext(ctx, 5, SortByCPU)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetTopProcessesWithContext() = %v, want context.Canceled", err)
	}
	if elapsed >= defaultCPUSampleWindow {
		t.Errorf("GetTopProcessesWithContext() took %v on a cancelled context, want less than the %v sample window", elapsed, defaultCPUSampleWindow)
	}
}

// The average-mode collector has no sleep to interrupt, so its
// cancellation check lives in the per-pid loop instead.
func TestGetTopProcessesAverageModeAbortsOnCancel(t *testing.T) {
	syStats := procFixtureStats() // already CPUUsageAverage

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := syStats.GetTopProcessesWithContext(ctx, 5, SortByCPU); !errors.Is(err, context.Canceled) {
		t.Errorf("GetTopProcessesWithContext() = %v, want context.Canceled", err)
	}
}

// The no-context methods must keep working exactly as before - that is
// what makes this change additive.
func TestLegacyMethodsStillWork(t *testing.T) {
	syStats := procFixtureStats()

	if _, err := syStats.GetTopProcesses(5, SortByCPU); err != nil {
		t.Errorf("GetTopProcesses() = %v, want nil", err)
	}
	if _, err := syStats.GetProcess(1234); err != nil {
		t.Errorf("GetProcess() = %v, want nil", err)
	}
	if _, err := cpuFixtureStats().GetCPU(); err != nil {
		t.Errorf("GetCPU() = %v, want nil", err)
	}
}

// TestConcurrentUse backs the thread-safety claim in the package docs with
// the race detector. Run as part of `go test -race`.
func TestConcurrentUse(t *testing.T) {
	syStats := procFixtureStats()
	cpu := cpuFixtureStats()
	cpu.CPUSampleWindow = time.Millisecond

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if _, err := syStats.GetTopProcesses(3, SortByCPU); err != nil {
					t.Errorf("GetTopProcesses: %v", err)
				}
				if _, err := syStats.GetProcess(1234); err != nil {
					t.Errorf("GetProcess: %v", err)
				}
				if _, err := syStats.GetMemory(Megabyte); err != nil {
					t.Errorf("GetMemory: %v", err)
				}
				if _, err := cpu.GetCPU(); err != nil {
					t.Errorf("GetCPU: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}
