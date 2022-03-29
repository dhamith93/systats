package systats_test

import (
	"strings"
	"testing"

	"github.com/dhamith93/systats"
	"github.com/dhamith93/systats/exec"
)

func TestGetMemoryKB(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo.txt"}
	got, err := syStats.GetMemory(systats.Kilobyte)
	if err != nil {
		t.Errorf("Get memory returned error")
	}

	if got.Available != 8210776 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Available, 8210776)
		return
	}

	if got.Free != 3921162 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Free, 3921162)
		return
	}

	if got.Used != 8949452 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Used, 8949452)
		return
	}

	if got.Total != 16706908 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Total, 16706908)
		return
	}

	if got.PercentageUsed != 53.56737892069672 {
		t.Errorf("Got invalid value. got: %f, want: %f", got.PercentageUsed, 53.56737892069672)
		return
	}
}

func TestGetMemoryMB(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo.txt"}
	got, err := syStats.GetMemory(systats.Megabyte)
	if err != nil {
		t.Errorf("Get memory returned error")
	}

	if got.Available != 8018 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Available, 8018)
		return
	}

	if got.Free != 3829 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Free, 3829)
		return
	}

	if got.Used != 8739 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Used, 8739)
		return
	}

	if got.Total != 16315 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Total, 16315)
		return
	}

	if got.PercentageUsed != 53.56737892069672 {
		t.Errorf("Got invalid value. got: %f, want: %f", got.PercentageUsed, 53.56737892069672)
		return
	}
}

func TestGetSwapKB(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo.txt"}
	got, err := syStats.GetSwap(systats.Kilobyte)
	if err != nil {
		t.Errorf("Get swap returned error")
	}

	if got.Free != 2065559 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Free, 2065559)
		return
	}

	if got.Used != 81920 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Used, 81920)
		return
	}

	if got.Total != 2147479 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Total, 2147479)
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

	if got.Free != 2017 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Free, 2017)
		return
	}

	if got.Used != 80 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Used, 80)
		return
	}

	if got.Total != 2097 {
		t.Errorf("Got invalid value. got: %d, want: %d", got.Total, 2097)
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
	}
	cpu, err := syStats.GetCPU()
	if err != nil {
		t.Errorf("Get CPU returned error")
	}

	if cpu.Model != "Intel(R) Core(TM) i3-10100F CPU @ 3.60GHz" {
		t.Errorf("Got invalid value. got: %s, want: %s", cpu.Model, "Intel(R) Core(TM) i3-10100F CPU @ 3.60GHz")
		return
	}

	if cpu.NoOfCores != 4 {
		t.Errorf("Got invalid value. got: %d, want: %d", cpu.NoOfCores, 4)
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
	_, err := syStats.GetNetworks()
	if err != nil {
		t.Errorf("Get Networks returned error %s", err.Error())
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

	if len(cpu) != 10 {
		t.Errorf("Got invalid value for process list length (cpu) got: %d, want: %d", len(cpu), 10)
	}

	if len(mem) != 10 {
		t.Errorf("Got invalid value for process list length (mem) got: %d, want: %d", len(mem), 10)
	}
}

func TestGetDisks(t *testing.T) {
	syStats := systats.New()
	_, err := syStats.GetDisks()
	if err != nil {
		t.Errorf("GetDisks() returned error %s", err.Error())
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
	disk.Convert(systats.Megabyte)
	if disk.Usage.Size != 112162 {
		t.Errorf("Got invalid value. got: %d, want: %d", disk.Usage.Size, 112162)
		return
	}
	if disk.Usage.Used != 102607 {
		t.Errorf("Got invalid value. got: %d, want: %d", disk.Usage.Used, 102607)
		return
	}
	if disk.Usage.Available != 3814 {
		t.Errorf("Got invalid value. got: %d, want: %d", disk.Usage.Available, 3814)
		return
	}

	disk.Convert(systats.Gigabyte)
	if disk.Usage.Size != 109 {
		t.Errorf("Got invalid value. got: %d, want: %d", disk.Usage.Size, 109)
		return
	}
	if disk.Usage.Used != 100 {
		t.Errorf("Got invalid value. got: %d, want: %d", disk.Usage.Used, 100)
		return
	}
	if disk.Usage.Available != 3 {
		t.Errorf("Got invalid value. got: %d, want: %d", disk.Usage.Available, 3)
		return
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
