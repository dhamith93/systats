package systats_test

import (
	"strings"
	"testing"

	"github.com/dhamith93/systats"
)

func TestGetMemoryKB(t *testing.T) {
	syStats := systats.SyStats{MeminfoPath: "./test_files/meminfo.txt"}
	got, err := systats.GetMemory(syStats, systats.Kilobyte)
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
	got, err := systats.GetMemory(syStats, systats.Megabyte)
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
	got, err := systats.GetSwap(syStats, systats.Kilobyte)
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
	got, err := systats.GetSwap(syStats, systats.Megabyte)
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
	cpu, err := systats.GetCPU(syStats)
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
	system, err := systats.GetSystem(syStats)
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

	if system.TimeZone != "Colombo/Asia" {
		t.Errorf("Got invalid value. got: %s, want: %s", system.TimeZone, "Colombo/Asia")
		return
	}
}

func TestGetNetworks(t *testing.T) {
	syStats := systats.New()
	_, err := systats.GetNetworks(syStats)
	if err != nil {
		t.Errorf("Get Networks returned error %s", err.Error())
	}
}

func TestIsServiceRunning(t *testing.T) {
	// gets first running service
	output := systats.ExecuteWithPipe("service --status-all | awk '$2 == \"+\" {print $4}' | head -n 1")
	output = strings.TrimSpace(output)
	running := systats.IsServiceRunning(output)
	if !running {
		t.Errorf("IsServiceRunning(%s) returned %v, expected %v", output, running, true)
	}
}

func TestGetTopProcesses(t *testing.T) {
	_, err := systats.GetTopProcesses(10, "cpu")
	if err != nil {
		t.Errorf("GetTopProcesses(CPU) returned error %s", err.Error())
	}

	_, err = systats.GetTopProcesses(10, "memory")
	if err != nil {
		t.Errorf("GetTopProcesses(MEMORY) returned error %s", err.Error())
	}
}
