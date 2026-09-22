package systats

import "testing"

func TestParsePressure(t *testing.T) {
	content := "some avg10=1.23 avg60=0.45 avg300=0.10 total=123456789\n" +
		"full avg10=0.50 avg60=0.20 avg300=0.05 total=9876543\n"

	got := parsePressure(content)

	if !got.FullAvailable {
		t.Errorf("FullAvailable = false, want true")
	}
	if got.Some.Avg10 != 1.23 || got.Some.Avg60 != 0.45 || got.Some.Avg300 != 0.10 {
		t.Errorf("Some averages = %+v, want 1.23/0.45/0.10", got.Some)
	}
	if got.Some.Total != 123456789 {
		t.Errorf("Some.Total = %d, want 123456789", got.Some.Total)
	}
	if got.Full.Avg10 != 0.50 || got.Full.Total != 9876543 {
		t.Errorf("Full = %+v, want avg10 0.50 and total 9876543", got.Full)
	}
}

// /proc/pressure/cpu has no "full" line on most kernels, so its zeros
// must be distinguishable from a real zero reading.
func TestParsePressureWithoutFullLine(t *testing.T) {
	got := parsePressure("some avg10=3.00 avg60=2.00 avg300=1.00 total=42\n")

	if got.FullAvailable {
		t.Errorf("FullAvailable = true, want false when the kernel reports no full line")
	}
	if got.Some.Avg10 != 3.00 {
		t.Errorf("Some.Avg10 = %v, want 3.00", got.Some.Avg10)
	}
	if got.Full != (PressureMetric{}) {
		t.Errorf("Full = %+v, want the zero value", got.Full)
	}
}

// The kernel has added fields to these files before; an unknown key must
// not break the known ones, and a malformed value must not take out the
// whole reading.
func TestParsePressureToleratesOddInput(t *testing.T) {
	got := parsePressure("some avg10=1.5 futurefield=9 avg60=notanumber total=7 noequalsign\n" +
		"garbage line here\n\n")

	if got.Some.Avg10 != 1.5 {
		t.Errorf("Some.Avg10 = %v, want 1.5", got.Some.Avg10)
	}
	if got.Some.Avg60 != 0 {
		t.Errorf("Some.Avg60 = %v, want 0 for an unparseable value", got.Some.Avg60)
	}
	if got.Some.Total != 7 {
		t.Errorf("Some.Total = %d, want 7", got.Some.Total)
	}
}

func TestGetPressure(t *testing.T) {
	syStats := &SyStats{PressurePath: "./test_files/pressure"}

	got, err := getPressure(syStats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !got.Available {
		t.Fatalf("Available = false, want true")
	}
	if got.Limited {
		t.Errorf("Limited = true, want false without ContainerAware")
	}
	if got.CPU.Some.Avg10 != 1.23 {
		t.Errorf("CPU.Some.Avg10 = %v, want 1.23", got.CPU.Some.Avg10)
	}
	if got.IO.Some.Total != 555444333 {
		t.Errorf("IO.Some.Total = %d, want 555444333", got.IO.Some.Total)
	}
	if got.Memory.Some.Avg10 != 0 {
		t.Errorf("Memory.Some.Avg10 = %v, want 0", got.Memory.Some.Avg10)
	}
	if got.Time == 0 {
		t.Errorf("Time = 0, want a timestamp")
	}
}

// A kernel without PSI (pre-4.20, or CONFIG_PSI=n) is a normal condition,
// not an error - the caller checks Available.
func TestGetPressureUnavailable(t *testing.T) {
	syStats := &SyStats{PressurePath: "./test_files/nonexistent_pressure"}

	got, err := getPressure(syStats)
	if err != nil {
		t.Fatalf("a kernel without PSI should not be an error, got %v", err)
	}
	if got.Available {
		t.Errorf("Available = true, want false when /proc/pressure is absent")
	}
}

func TestGetPressureOlderKernelCPUHasNoFull(t *testing.T) {
	syStats := &SyStats{PressurePath: "./test_files/pressure_no_full"}

	got, err := getPressure(syStats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !got.Available {
		t.Fatalf("Available = false, want true")
	}
	if got.CPU.FullAvailable {
		t.Errorf("CPU.FullAvailable = true, want false for this fixture")
	}
	if !got.Memory.FullAvailable {
		t.Errorf("Memory.FullAvailable = false, want true - memory does report full")
	}
}

// ContainerAware falls back to host-wide when there's no cgroup v2
// pressure to read, matching how GetMemory/GetCPU behave.
func TestGetPressureContainerAwareFallsBackToHost(t *testing.T) {
	syStats := &SyStats{
		PressurePath:   "./test_files/pressure",
		ContainerAware: true,
		CgroupRootPath: "./test_files/cgroup_v1",
		SelfCgroupPath: "./test_files/cgroup_v1/self_cgroup.txt",
	}

	got, err := getPressure(syStats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Limited {
		t.Errorf("Limited = true, want false - cgroup v1 has no PSI equivalent")
	}
	if got.CPU.Some.Avg10 != 1.23 {
		t.Errorf("CPU.Some.Avg10 = %v, want the host-wide 1.23", got.CPU.Some.Avg10)
	}
}

// With cgroup v2 PSI present, the figures come from the calling process's
// own cgroup rather than the host - the point of the feature inside a
// container.
func TestGetPressureContainerAwareUsesCgroup(t *testing.T) {
	syStats := &SyStats{
		PressurePath:   "./test_files/pressure",
		ContainerAware: true,
		CgroupRootPath: "./test_files/cgroup_v2",
		SelfCgroupPath: "./test_files/cgroup_v2/self_cgroup.txt",
	}

	got, err := getPressure(syStats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !got.Limited {
		t.Fatalf("Limited = false, want true when cgroup v2 pressure is readable")
	}
	// The cgroup fixture says 42.00; the host fixture says 1.23.
	if got.CPU.Some.Avg10 != 42.00 {
		t.Errorf("CPU.Some.Avg10 = %v, want the cgroup's 42.00 (host says 1.23)", got.CPU.Some.Avg10)
	}
	if got.Memory.Some.Total != 111 {
		t.Errorf("Memory.Some.Total = %d, want the cgroup's 111", got.Memory.Some.Total)
	}
}

// ContainerAware off must ignore the cgroup even when it's fully readable.
func TestGetPressureContainerAwareFalseIgnoresCgroup(t *testing.T) {
	syStats := &SyStats{
		PressurePath:   "./test_files/pressure",
		CgroupRootPath: "./test_files/cgroup_v2",
		SelfCgroupPath: "./test_files/cgroup_v2/self_cgroup.txt",
	}

	got, err := getPressure(syStats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Limited {
		t.Errorf("Limited = true, want false when ContainerAware is off")
	}
	if got.CPU.Some.Avg10 != 1.23 {
		t.Errorf("CPU.Some.Avg10 = %v, want the host-wide 1.23", got.CPU.Some.Avg10)
	}
}
