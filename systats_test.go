package systats_test

import (
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
