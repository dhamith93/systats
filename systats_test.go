package systats_test

import (
	"strings"
	"testing"

	"github.com/dhamith93/systats"
	"github.com/dhamith93/systats/exec"
	"github.com/dhamith93/systats/internal/unitconv"
)

func TestGetMemoryKB(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo.txt"}
	got, err := syStats.GetMemory(systats.Kilobyte)
	if err != nil {
		t.Errorf("Get memory returned error")
	}

	if got.Available != 8018336 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Available, 8018336)
		return
	}

	if got.Free != 3829260 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Free, 3829260)
		return
	}

	if got.Used != 8739700 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Used, 8739700)
		return
	}

	if got.Total != 16315340 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Total, 16315340)
		return
	}

	if got.PercentageUsed != 53.56737892069672 {
		t.Errorf("Got invalid value. got: %f, want: %f", got.PercentageUsed, 53.56737892069672)
		return
	}
}

// TestGetMemoryRecoversFromPanic proves that a malformed /proc value
// (which makes strops.ToUint64 panic internally) surfaces as a normal
// error from GetMemory instead of crashing the caller.
func TestGetMemoryRecoversFromPanic(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo_corrupt.txt"}
	_, err := syStats.GetMemory(systats.Kilobyte)
	if err == nil {
		t.Errorf("GetMemory() with corrupt input returned nil error, want a recovered-panic error")
	}
}

func TestGetMemoryMB(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo.txt"}
	got, err := syStats.GetMemory(systats.Megabyte)
	if err != nil {
		t.Errorf("Get memory returned error")
	}

	if got.Available != 8018336.0/1024 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Available, 8018336.0/1024)
		return
	}

	if got.Free != 3829260.0/1024 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Free, 3829260.0/1024)
		return
	}

	if got.Used != 8739700.0/1024 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Used, 8739700.0/1024)
		return
	}

	if got.Total != 16315340.0/1024 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Total, 16315340.0/1024)
		return
	}

	if got.PercentageUsed != 53.56737892069672 {
		t.Errorf("Got invalid value. got: %f, want: %f", got.PercentageUsed, 53.56737892069672)
		return
	}
}

func TestNewDefaultsToContainerAwareFalse(t *testing.T) {
	syStats := systats.New()
	if syStats.ContainerAware {
		t.Errorf("New().ContainerAware = true, want false (existing callers must keep today's host-wide behavior)")
	}
}

func TestGetMemoryContainerAwareV2Limited(t *testing.T) {
	syStats := systats.SyStats{
		MeminfoPath:    "./test_files/meminfo.txt",
		ContainerAware: true,
		CgroupRootPath: "./test_files/cgroup_v2",
		SelfCgroupPath: "./test_files/cgroup_v2/self_cgroup.txt",
	}
	got, err := syStats.GetMemory(systats.Kilobyte)
	if err != nil {
		t.Fatalf("GetMemory() returned error: %s", err.Error())
	}

	if !got.Limited {
		t.Fatalf("Limited = false, want true")
	}

	wantTotal := unitconv.KibToKB(536870912 / 1024)
	wantUsed := unitconv.KibToKB((104857600 - 10485760) / 1024)
	if got.Total != wantTotal {
		t.Errorf("Total = %v, want %v", got.Total, wantTotal)
	}
	if got.Used != wantUsed {
		t.Errorf("Used = %v, want %v", got.Used, wantUsed)
	}
	if got.Available != got.Free {
		t.Errorf("Available (%v) and Free (%v) should be equal when Limited", got.Available, got.Free)
	}
	if got.Available != got.Total-got.Used {
		t.Errorf("Available = %v, want Total-Used = %v", got.Available, got.Total-got.Used)
	}
}

func TestGetMemoryContainerAwareV1Limited(t *testing.T) {
	syStats := systats.SyStats{
		MeminfoPath:    "./test_files/meminfo.txt",
		ContainerAware: true,
		CgroupRootPath: "./test_files/cgroup_v1",
		SelfCgroupPath: "./test_files/cgroup_v1/self_cgroup.txt",
	}
	got, err := syStats.GetMemory(systats.Kilobyte)
	if err != nil {
		t.Fatalf("GetMemory() returned error: %s", err.Error())
	}

	if !got.Limited {
		t.Fatalf("Limited = false, want true")
	}

	wantTotal := unitconv.KibToKB(536870912 / 1024)
	wantUsed := unitconv.KibToKB((104857600 - 10485760) / 1024) // proves total_inactive_file (not inactive_file) was used
	if got.Total != wantTotal {
		t.Errorf("Total = %v, want %v", got.Total, wantTotal)
	}
	if got.Used != wantUsed {
		t.Errorf("Used = %v, want %v", got.Used, wantUsed)
	}
}

func TestGetMemoryContainerAwareUnlimitedFallsBackToHost(t *testing.T) {
	syStats := systats.SyStats{
		MeminfoPath:    "./test_files/meminfo.txt",
		ContainerAware: true,
		CgroupRootPath: "./test_files/cgroup_v2_unlimited",
		SelfCgroupPath: "./test_files/cgroup_v2_unlimited/self_cgroup.txt",
	}
	got, err := syStats.GetMemory(systats.Megabyte)
	if err != nil {
		t.Fatalf("GetMemory() returned error: %s", err.Error())
	}

	if got.Limited {
		t.Errorf("Limited = true, want false when the cgroup has no configured limit")
	}
	// Should exactly match the plain host-wide TestGetMemoryMB values -
	// no cgroup limit means no override at all.
	if got.Total != 16315340.0/1024 {
		t.Errorf("Total = %v, want 15932 (unchanged host-wide value)", got.Total)
	}
}

func TestGetMemoryContainerAwareFalseIgnoresCgroup(t *testing.T) {
	syStats := systats.SyStats{
		MeminfoPath: "./test_files/meminfo.txt",
		// ContainerAware left false (zero value) despite pointing at a
		// fully valid, limited cgroup fixture - proves the feature is
		// truly opt-in.
		CgroupRootPath: "./test_files/cgroup_v2",
		SelfCgroupPath: "./test_files/cgroup_v2/self_cgroup.txt",
	}
	got, err := syStats.GetMemory(systats.Megabyte)
	if err != nil {
		t.Fatalf("GetMemory() returned error: %s", err.Error())
	}

	if got.Limited {
		t.Errorf("Limited = true, want false when ContainerAware is false")
	}
	if got.Total != 16315340.0/1024 {
		t.Errorf("Total = %v, want 15932 (unchanged host-wide value)", got.Total)
	}
}

func TestGetSwapKB(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo.txt"}
	got, err := syStats.GetSwap(systats.Kilobyte)
	if err != nil {
		t.Errorf("Get swap returned error")
	}

	if got.Free != 2017148 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Free, 2017148)
		return
	}

	if got.Used != 80000 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Used, 80000)
		return
	}

	if got.Total != 2097148 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Total, 2097148)
		return
	}

	if got.PercentageUsed != 3.814704541596492 {
		t.Errorf("Got invalid value. got: %f, want: %f", got.PercentageUsed, 3.814704541596492)
		return
	}
}

func TestGetSwapMB(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo.txt"}
	got, err := syStats.GetSwap(systats.Megabyte)
	if err != nil {
		t.Errorf("Get swap returned error")
	}

	if got.Free != 2017148.0/1024 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Free, 2017148.0/1024)
		return
	}

	if got.Used != 80000.0/1024 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Used, 80000.0/1024)
		return
	}

	if got.Total != 2097148.0/1024 {
		t.Errorf("Got invalid value. got: %v, want: %v", got.Total, 2097148.0/1024)
		return
	}

	if got.PercentageUsed != 3.814704541596492 {
		t.Errorf("Got invalid value. got: %f, want: %f", got.PercentageUsed, 3.814704541596492)
		return
	}
}

func TestGetCPU(t *testing.T) {
	syStats := systats.SyStats{
		CPUinfoFilePath: "./test_files/cpuinfo.txt",
		StatFilePath:    "/proc/stat",
		LoadAvgPath:     "./test_files/loadavg.txt",
	}
	cpu, err := syStats.GetCPU()
	if err != nil {
		t.Errorf("Get CPU returned error")
	}

	if cpu.Model != "Intel(R) Core(TM) i3-10100F CPU @ 3.60GHz" {
		t.Errorf("Got invalid value. got: %s, want: %s", cpu.Model, "Intel(R) Core(TM) i3-10100F CPU @ 3.60GHz")
		return
	}

	// NoOfCores is logical CPUs: the fixture has 8 processor entries, even
	// though its "cpu cores" field says 4 (that's physical-per-socket).
	if cpu.NoOfCores != 8 {
		t.Errorf("Got invalid value. got: %d, want: %d", cpu.NoOfCores, 8)
		return
	}

	if cpu.PhysicalCores != 4 {
		t.Errorf("Got invalid value. got: %d, want: %d", cpu.PhysicalCores, 4)
		return
	}

	if cpu.Sockets != 1 {
		t.Errorf("Got invalid value. got: %d, want: %d", cpu.Sockets, 1)
		return
	}

	if cpu.Load1 != 0.52 {
		t.Errorf("Got invalid value. got: %f, want: %f", cpu.Load1, 0.52)
		return
	}

	if cpu.Load5 != 0.58 {
		t.Errorf("Got invalid value. got: %f, want: %f", cpu.Load5, 0.58)
		return
	}

	if cpu.Load15 != 0.59 {
		t.Errorf("Got invalid value. got: %f, want: %f", cpu.Load15, 0.59)
		return
	}
}

func TestGetSystem(t *testing.T) {
	syStats := systats.SyStats{
		EtcPath:     "./test_files/",
		VersionPath: "./test_files/version.txt",
		UptimePath:  "./test_files/uptime.txt",
	}
	system, err := syStats.GetSystem()
	if err != nil {
		t.Errorf("Get System returned error %s", err.Error())
	}

	if system.OS != "Ubuntu 21.10" {
		t.Errorf("Got invalid value. got: %s, want: %s", system.OS, "Ubuntu 21.10")
		return
	}

	if system.Kernel != "5.13.0-30-generic" {
		t.Errorf("Got invalid value. got: %s, want: %s", system.Kernel, "5.13.0-30-generic")
		return
	}

	if system.UpTime != "3h20m58s" {
		t.Errorf("Got invalid value. got: %s, want: %s", system.UpTime, "3h20m58s")
		return
	}

	if system.TimeZone != "Asia/Colombo" {
		t.Errorf("Got invalid value. got: %s, want: %s", system.TimeZone, "Asia/Colombo")
		return
	}
}

func TestGetNetworks(t *testing.T) {
	syStats := systats.New()
	networks, err := syStats.GetNetworks()
	if err != nil {
		t.Errorf("Get Networks returned error %s", err.Error())
	}

	for _, n := range networks {
		if n.Interface == "" {
			t.Errorf("Got network entry with empty Interface name")
		}
		if n.Interface == "lo" {
			t.Errorf("Got loopback interface %q in GetNetworks() result, want it excluded", n.Interface)
		}
	}
}

func TestGetNetworkUsage(t *testing.T) {
	syStats := systats.New()
	n, err := syStats.GetNetworks()
	if err != nil {
		t.Errorf("Get Network Usage returned error %s", err.Error())
	}

	if len(n) > 0 {
		out := syStats.GetNetworkUsage(n[0].Interface)
		if out.RxBytes == 0 {
			t.Errorf("Got invalid value for Rx. got: %d, want: > %d", out.RxBytes, 0)
		}

		if out.TxBytes == 0 {
			t.Errorf("Got invalid value for Tx. got: %d, want: > %d", out.TxBytes, 0)
		}
	}
}

func TestIsServiceRunning(t *testing.T) {
	// gets first running service

	output := exec.ExecuteWithPipe("service --status-all | awk '$2 == \"+\" {print $4}' | head -n 1")
	output = strings.TrimSpace(output)
	syStats := systats.New()
	running := syStats.IsServiceRunning(output)
	if !running {
		t.Errorf("IsServiceRunning(%s) returned %v, expected %v", output, running, true)
	}
}

func TestNewDefaultsToInstantCPUMode(t *testing.T) {
	syStats := systats.New()
	if syStats.ProcessCPUMode != systats.CPUUsageInstant {
		t.Errorf("New().ProcessCPUMode = %q, want %q (existing callers must keep today's behavior)", syStats.ProcessCPUMode, systats.CPUUsageInstant)
	}
}

func TestGetTopProcesses(t *testing.T) {
	syStats := systats.New()
	cpu, err := syStats.GetTopProcesses(10, "cpu")
	if err != nil {
		t.Errorf("GetTopProcesses(CPU) returned error %s", err.Error())
	}

	mem, err := syStats.GetTopProcesses(10, "memory")
	if err != nil {
		t.Errorf("GetTopProcesses(MEMORY) returned error %s", err.Error())
	}

	if len(cpu) == 0 || len(cpu) > 10 {
		t.Errorf("Got invalid value for process list length (cpu) got: %d, want: > 0 and <= %d", len(cpu), 10)
	}

	if len(mem) == 0 || len(mem) > 10 {
		t.Errorf("Got invalid value for process list length (mem) got: %d, want: > 0 and <= %d", len(mem), 10)
	}

	for i := 1; i < len(cpu); i++ {
		if cpu[i-1].CPUUsage < cpu[i].CPUUsage {
			t.Errorf("GetTopProcesses(cpu) not sorted descending by CPUUsage at position %d: %+v", i, cpu)
			break
		}
	}

	for i := 1; i < len(mem); i++ {
		if mem[i-1].MemUsage < mem[i].MemUsage {
			t.Errorf("GetTopProcesses(memory) not sorted descending by MemUsage at position %d: %+v", i, mem)
			break
		}
	}
}

func TestGetDisks(t *testing.T) {
	syStats := systats.New()
	_, err := syStats.GetDisks()
	if err != nil {
		t.Errorf("GetDisks() returned error %s", err.Error())
	}
}

func TestGetDisksExcludesPseudoFilesystems(t *testing.T) {
	syStats := systats.SyStats{MountsPath: "./test_files/mounts.txt"}
	disks, err := syStats.GetDisks()
	if err != nil {
		t.Errorf("GetDisks() returned error %s", err.Error())
	}

	for _, d := range disks {
		if d.Type == "tmpfs" || d.Type == "devtmpfs" || d.Type == "udev" {
			t.Errorf("got excluded fs type %q in GetDisks() result for %s", d.Type, d.MountedOn)
		}
		if d.MountedOn == "" {
			t.Errorf("got disk entry with empty MountedOn")
		}
		if d.Usage.Unit != systats.Byte {
			t.Errorf("got Usage.Unit %q, want %q", d.Usage.Unit, systats.Byte)
		}
	}
}

func TestDiskConvert(t *testing.T) {
	disk := systats.Disk{
		FileSystem: "TEST",
		Type:       "TEST",
		MountedOn:  "TEST",
		Usage: systats.DiskUsage{
			Size:      117610516480,
			Used:      107592122368,
			Available: 3999989760,
			Usage:     "97%",
			Unit:      systats.Byte,
		},
	}
	// Written as exact divisions rather than decimal literals: every
	// divisor is a power of two, so these are exact in float64. As
	// integers these truncated to 112162 / 102607 / 3814.
	if err := disk.Convert(systats.Megabyte); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := 117610516480.0 / 1024 / 1024; disk.Usage.Size != want {
		t.Errorf("Got invalid value. got: %v, want: %v", disk.Usage.Size, want)
		return
	}
	if want := 107592122368.0 / 1024 / 1024; disk.Usage.Used != want {
		t.Errorf("Got invalid value. got: %v, want: %v", disk.Usage.Used, want)
		return
	}
	if want := 3999989760.0 / 1024 / 1024; disk.Usage.Available != want {
		t.Errorf("Got invalid value. got: %v, want: %v", disk.Usage.Available, want)
		return
	}

	if err := disk.Convert(systats.Gigabyte); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := 117610516480.0 / 1024 / 1024 / 1024; disk.Usage.Size != want {
		t.Errorf("Got invalid value. got: %v, want: %v", disk.Usage.Size, want)
		return
	}
	if want := 107592122368.0 / 1024 / 1024 / 1024; disk.Usage.Used != want {
		t.Errorf("Got invalid value. got: %v, want: %v", disk.Usage.Used, want)
		return
	}
	if want := 3999989760.0 / 1024 / 1024 / 1024; disk.Usage.Available != want {
		t.Errorf("Got invalid value. got: %v, want: %v", disk.Usage.Available, want)
		return
	}
}

// TestDiskConvertRoundTrips is the regression test for the integer
// truncation: as uint64 these fields lost 818 MB on a B -> GB -> B trip.
func TestDiskConvertRoundTrips(t *testing.T) {
	const sizeBytes = 500107862016.0

	disk := systats.Disk{
		Usage: systats.DiskUsage{
			Size: sizeBytes,
			Unit: systats.Byte,
		},
	}

	if err := disk.Convert(systats.Gigabyte); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := disk.Convert(systats.Byte); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if disk.Usage.Size != sizeBytes {
		t.Errorf("round trip B -> GB -> B = %v, want %v (lost %v bytes)",
			disk.Usage.Size, sizeBytes, sizeBytes-disk.Usage.Size)
	}
}

// Convert used to leave the figures unconverted but still stamp the new
// unit on them, so the struct reported values in a unit they weren't in.
func TestDiskConvertRejectsUnknownUnit(t *testing.T) {
	disk := systats.Disk{
		Usage: systats.DiskUsage{
			Size:      1024,
			Used:      512,
			Available: 512,
			Unit:      systats.Byte,
		},
	}
	before := disk.Usage

	if err := disk.Convert("XB"); err == nil {
		t.Errorf("Convert(\"XB\") returned nil error, want a rejection")
	}
	if disk.Usage != before {
		t.Errorf("Convert with a bad unit modified the struct: got %+v, want %+v", disk.Usage, before)
	}
}

// The unit the disk is currently in has to be recognized too, or the
// conversion factor would be a silent guess.
func TestDiskConvertRejectsUnknownSourceUnit(t *testing.T) {
	disk := systats.Disk{
		Usage: systats.DiskUsage{Size: 1024, Unit: "XB"},
	}

	if err := disk.Convert(systats.Megabyte); err == nil {
		t.Errorf("Convert from an unrecognized unit returned nil error, want a rejection")
	}
	if disk.Usage.Size != 1024 {
		t.Errorf("Size = %v, want it left untouched at 1024", disk.Usage.Size)
	}
}

// A partition smaller than the target unit used to report 0.
func TestDiskConvertKeepsSmallValues(t *testing.T) {
	disk := systats.Disk{
		Usage: systats.DiskUsage{
			Size: 512 * 1024 * 1024, // 512 MiB
			Unit: systats.Byte,
		},
	}

	if err := disk.Convert(systats.Gigabyte); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := 0.5; disk.Usage.Size != want {
		t.Errorf("512 MiB in GiB = %v, want %v", disk.Usage.Size, want)
	}
}

func TestIsPortOpen(t *testing.T) {
	syStats := systats.New()
	status := syStats.IsPortOpen(0000)
	want := false
	if status != want {
		t.Errorf("Got invalid value. got: %v, want: %v", status, want)
	}
}

func TestCanConnect(t *testing.T) {
	syStats := systats.New()
	status, err := syStats.CanConnectExternal("https://www.google.com")
	if err != nil {
		t.Errorf("CanConnect() returned error %s", err.Error())
	}
	want := true
	if status != want {
		t.Errorf("Got invalid value. got: %v, want: %v", status, want)
	}
}

// TestGetMemoryAllUnits covers Byte and Gigabyte, which were exported
// constants the API used to reject.
func TestGetMemoryAllUnits(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo.txt"}

	// Float division, deliberately: Gigabyte used to truncate 15.55 to 15.
	cases := map[systats.Unit]float64{
		systats.Byte:     16315340 * 1024,
		systats.Kilobyte: 16315340,
		systats.Megabyte: 16315340.0 / 1024,
		systats.Gigabyte: 16315340.0 / 1024 / 1024,
	}
	for unit, wantTotal := range cases {
		got, err := syStats.GetMemory(unit)
		if err != nil {
			t.Errorf("GetMemory(%q) returned error %s", unit, err.Error())
			continue
		}
		if got.Total != wantTotal {
			t.Errorf("GetMemory(%q).Total = %v, want %v", unit, got.Total, wantTotal)
		}
		if got.Unit != unit {
			t.Errorf("GetMemory(%q).Unit = %q", unit, got.Unit)
		}
	}

	if _, err := syStats.GetMemory("furlongs"); err == nil {
		t.Errorf("expected an error for an unsupported unit")
	}
}

func TestGetSwapAllUnits(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo.txt"}
	for _, unit := range []systats.Unit{systats.Byte, systats.Kilobyte, systats.Megabyte, systats.Gigabyte} {
		if _, err := syStats.GetSwap(unit); err != nil {
			t.Errorf("GetSwap(%q) returned error %s", unit, err.Error())
		}
	}
	if _, err := syStats.GetSwap("furlongs"); err == nil {
		t.Errorf("expected an error for an unsupported unit")
	}
}
