package systats

import "testing"

func TestDetectCgroupVersion(t *testing.T) {
	cases := []struct {
		name string
		root string
		want cgroupVersion
	}{
		{"v2", "./test_files/cgroup_v2", cgroupV2},
		{"v1", "./test_files/cgroup_v1", cgroupV1},
		{"none", "./test_files/does_not_exist", cgroupNone},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detectCgroupVersion(c.root)
			if got != c.want {
				t.Errorf("detectCgroupVersion(%q) = %v, want %v", c.root, got, c.want)
			}
		})
	}
}

func TestParseSelfCgroupV2(t *testing.T) {
	got, err := parseSelfCgroupV2("0::/foo\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/foo" {
		t.Errorf("got %q, want %q", got, "/foo")
	}

	if _, err := parseSelfCgroupV2("garbage"); err == nil {
		t.Errorf("expected error for malformed content, got nil")
	}
}

func TestParseSelfCgroupV1(t *testing.T) {
	content := "11:memory:/\n5:cpu,cpuacct:/\n"

	cgPath, controllerDir, err := parseSelfCgroupV1(content, "memory")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cgPath != "/" || controllerDir != "memory" {
		t.Errorf("memory: got (%q, %q), want (\"/\", \"memory\")", cgPath, controllerDir)
	}

	for _, controller := range []string{"cpu", "cpuacct"} {
		cgPath, controllerDir, err := parseSelfCgroupV1(content, controller)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", controller, err)
		}
		if cgPath != "/" || controllerDir != "cpu,cpuacct" {
			t.Errorf("%s: got (%q, %q), want (\"/\", \"cpu,cpuacct\")", controller, cgPath, controllerDir)
		}
	}

	if _, _, err := parseSelfCgroupV1(content, "cpuset"); err == nil {
		t.Errorf("expected error for missing controller, got nil")
	}
}

func TestResolveCgroupControllerPath(t *testing.T) {
	v2 := &SyStats{CgroupRootPath: "./test_files/cgroup_v2", SelfCgroupPath: "./test_files/cgroup_v2/self_cgroup.txt"}
	dir, ok := resolveCgroupControllerPath(v2, cgroupV2, "")
	if !ok || dir != "test_files/cgroup_v2" {
		t.Errorf("v2: got (%q, %v), want (\"test_files/cgroup_v2\", true)", dir, ok)
	}

	v1 := &SyStats{CgroupRootPath: "./test_files/cgroup_v1", SelfCgroupPath: "./test_files/cgroup_v1/self_cgroup.txt"}
	dir, ok = resolveCgroupControllerPath(v1, cgroupV1, "memory")
	if !ok || dir != "test_files/cgroup_v1/memory" {
		t.Errorf("v1 memory: got (%q, %v), want (\"test_files/cgroup_v1/memory\", true)", dir, ok)
	}
	dir, ok = resolveCgroupControllerPath(v1, cgroupV1, "cpuacct")
	if !ok || dir != "test_files/cgroup_v1/cpu,cpuacct" {
		t.Errorf("v1 cpuacct: got (%q, %v), want (\"test_files/cgroup_v1/cpu,cpuacct\", true)", dir, ok)
	}
}

func TestReadCgroupMemoryLimit(t *testing.T) {
	cases := []struct {
		name        string
		version     cgroupVersion
		dir         string
		wantLimit   uint64
		wantLimited bool
	}{
		{"v2 limited", cgroupV2, "./test_files/cgroup_v2", 536870912, true},
		{"v2 unlimited", cgroupV2, "./test_files/cgroup_v2_unlimited", 0, false},
		{"v1 limited", cgroupV1, "./test_files/cgroup_v1/memory", 536870912, true},
		{"v1 unlimited", cgroupV1, "./test_files/cgroup_v1_unlimited/memory", 0, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			limit, limited, err := readCgroupMemoryLimit(c.version, c.dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if limited != c.wantLimited {
				t.Errorf("limited = %v, want %v", limited, c.wantLimited)
			}
			if c.wantLimited && limit != c.wantLimit {
				t.Errorf("limit = %d, want %d", limit, c.wantLimit)
			}
		})
	}
}

func TestReadCgroupMemoryUsage(t *testing.T) {
	usage, err := readCgroupMemoryUsage(cgroupV2, "./test_files/cgroup_v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage != 104857600 {
		t.Errorf("v2 usage = %d, want %d", usage, 104857600)
	}

	usage, err = readCgroupMemoryUsage(cgroupV1, "./test_files/cgroup_v1/memory")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage != 104857600 {
		t.Errorf("v1 usage = %d, want %d", usage, 104857600)
	}
}

func TestReadCgroupMemoryInactiveFileBytes(t *testing.T) {
	// v2 has only "inactive_file".
	got := readCgroupMemoryInactiveFileBytes(cgroupV2, "./test_files/cgroup_v2")
	if got != 10485760 {
		t.Errorf("v2 = %d, want %d", got, 10485760)
	}

	// v1 fixture has both inactive_file (9999999) and total_inactive_file
	// (10485760) with different values specifically to prove
	// total_inactive_file wins.
	got = readCgroupMemoryInactiveFileBytes(cgroupV1, "./test_files/cgroup_v1/memory")
	if got != 10485760 {
		t.Errorf("v1 = %d, want %d (total_inactive_file should be preferred)", got, 10485760)
	}
}

func TestApplyCgroupMemory(t *testing.T) {
	cases := []struct {
		name        string
		systats     *SyStats
		wantLimited bool
		wantTotal   uint64 // KiB
		wantUsed    uint64 // KiB
	}{
		{
			name: "v2 limited",
			systats: &SyStats{
				ContainerAware: true,
				CgroupRootPath: "./test_files/cgroup_v2",
				SelfCgroupPath: "./test_files/cgroup_v2/self_cgroup.txt",
			},
			wantLimited: true,
			wantTotal:   536870912 / 1024,
			wantUsed:    (104857600 - 10485760) / 1024,
		},
		{
			name: "v1 limited",
			systats: &SyStats{
				ContainerAware: true,
				CgroupRootPath: "./test_files/cgroup_v1",
				SelfCgroupPath: "./test_files/cgroup_v1/self_cgroup.txt",
			},
			wantLimited: true,
			wantTotal:   536870912 / 1024,
			wantUsed:    (104857600 - 10485760) / 1024,
		},
		{
			name: "v2 unlimited falls back",
			systats: &SyStats{
				ContainerAware: true,
				CgroupRootPath: "./test_files/cgroup_v2_unlimited",
				SelfCgroupPath: "./test_files/cgroup_v2_unlimited/self_cgroup.txt",
			},
			wantLimited: false,
		},
		{
			name: "ContainerAware false ignores a fully valid limited fixture",
			systats: &SyStats{
				ContainerAware: false,
				CgroupRootPath: "./test_files/cgroup_v2",
				SelfCgroupPath: "./test_files/cgroup_v2/self_cgroup.txt",
			},
			wantLimited: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &memoryKiB{total: 999, used: 111} // sentinel host-wide values
			applyCgroupMemory(m, c.systats)

			if m.limited != c.wantLimited {
				t.Errorf("limited = %v, want %v", m.limited, c.wantLimited)
			}
			if !c.wantLimited {
				if m.total != 999 || m.used != 111 {
					t.Errorf("expected host-wide sentinel values untouched, got total=%d used=%d", m.total, m.used)
				}
				return
			}
			if m.total != c.wantTotal {
				t.Errorf("total = %d KiB, want %d KiB", m.total, c.wantTotal)
			}
			if m.used != c.wantUsed {
				t.Errorf("used = %d KiB, want %d KiB", m.used, c.wantUsed)
			}
			if m.available != m.free {
				t.Errorf("available (%d) and free (%d) should be equal when limited", m.available, m.free)
			}
			if m.available != m.total-m.used {
				t.Errorf("available = %d, want total-used = %d", m.available, m.total-m.used)
			}
		})
	}
}

func TestReadCgroupCPUQuota(t *testing.T) {
	cases := []struct {
		name      string
		version   cgroupVersion
		dir       string
		wantCores float64
		wantOK    bool
	}{
		{"v2 limited (0.5 cores)", cgroupV2, "./test_files/cgroup_v2", 0.5, true},
		{"v2 unlimited", cgroupV2, "./test_files/cgroup_v2_unlimited", 0, false},
		{"v1 limited (0.5 cores)", cgroupV1, "./test_files/cgroup_v1/cpu,cpuacct", 0.5, true},
		{"v1 unlimited", cgroupV1, "./test_files/cgroup_v1_unlimited/cpu,cpuacct", 0, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cores, limited, err := readCgroupCPUQuota(c.version, c.dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if limited != c.wantOK {
				t.Errorf("limited = %v, want %v", limited, c.wantOK)
			}
			if c.wantOK && cores != c.wantCores {
				t.Errorf("cores = %v, want %v", cores, c.wantCores)
			}
		})
	}
}

func TestParseCPUStatUsageUsec(t *testing.T) {
	value, ok := parseCPUStatUsageUsec("usage_usec 1234567\nuser_usec 1000000\n")
	if !ok || value != 1234567 {
		t.Errorf("got (%d, %v), want (1234567, true)", value, ok)
	}

	_, ok = parseCPUStatUsageUsec("user_usec 1000000\n")
	if ok {
		t.Errorf("expected ok=false when usage_usec is missing")
	}
}

func TestApplyCgroupCPU(t *testing.T) {
	info := cgroupCPUInfo{ok: true, limited: true, cores: 0.5}

	output := &CPU{LoadAvg: 999, CoreAvg: []int{1, 2, 3}}
	// 0.3 cores used out of 0.5 allocated over 1 second => 60%
	applyCgroupCPU(output, info, 10.0, 10.3, true, true, 1.0)

	if !output.Limited {
		t.Errorf("Limited = false, want true")
	}
	if output.AllocatedCores != 0.5 {
		t.Errorf("AllocatedCores = %v, want 0.5", output.AllocatedCores)
	}
	if output.LoadAvg != 60 {
		t.Errorf("LoadAvg = %d, want 60", output.LoadAvg)
	}
	if output.CoreAvg != nil {
		t.Errorf("CoreAvg = %v, want nil", output.CoreAvg)
	}

	// Unlimited: Limited/AllocatedCores set, but host-wide LoadAvg/CoreAvg
	// left untouched.
	unlimitedInfo := cgroupCPUInfo{ok: true, limited: false, cores: 0}
	output2 := &CPU{LoadAvg: 42, CoreAvg: []int{1, 2}}
	applyCgroupCPU(output2, unlimitedInfo, 0, 0, true, true, 1.0)
	if output2.Limited {
		t.Errorf("Limited = true, want false")
	}
	if output2.LoadAvg != 42 || output2.CoreAvg == nil {
		t.Errorf("host-wide LoadAvg/CoreAvg should be untouched when not limited, got LoadAvg=%d CoreAvg=%v", output2.LoadAvg, output2.CoreAvg)
	}

	// !info.ok: nothing touched at all.
	output3 := &CPU{LoadAvg: 7}
	applyCgroupCPU(output3, cgroupCPUInfo{ok: false}, 0, 0, true, true, 1.0)
	if output3.Limited || output3.LoadAvg != 7 {
		t.Errorf("expected no changes when info.ok is false, got %+v", output3)
	}
}

func TestResolveCgroupCPU(t *testing.T) {
	v2 := &SyStats{ContainerAware: true, CgroupRootPath: "./test_files/cgroup_v2", SelfCgroupPath: "./test_files/cgroup_v2/self_cgroup.txt"}
	info := resolveCgroupCPU(v2)
	if !info.ok || !info.limited || info.cores != 0.5 {
		t.Errorf("v2: got %+v, want ok=true limited=true cores=0.5", info)
	}

	v1 := &SyStats{ContainerAware: true, CgroupRootPath: "./test_files/cgroup_v1", SelfCgroupPath: "./test_files/cgroup_v1/self_cgroup.txt"}
	info = resolveCgroupCPU(v1)
	if !info.ok || !info.limited || info.cores != 0.5 {
		t.Errorf("v1: got %+v, want ok=true limited=true cores=0.5", info)
	}

	none := &SyStats{ContainerAware: true, CgroupRootPath: "./test_files/does_not_exist", SelfCgroupPath: "./test_files/does_not_exist/self_cgroup.txt"}
	info = resolveCgroupCPU(none)
	if info.ok {
		t.Errorf("expected ok=false for a nonexistent cgroup root, got %+v", info)
	}
}
