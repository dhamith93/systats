package systats

import (
	"testing"

	"github.com/dhamith93/systats/internal/fileops"
)

func parseDiskStatsFixture(t *testing.T, path string) map[string]DiskIO {
	t.Helper()
	byDevice := map[string]DiskIO{}
	for _, d := range parseDiskStats(fileops.ReadFile(path)) {
		byDevice[d.Device] = d
	}
	return byDevice
}

func TestParseDiskStats14Field(t *testing.T) {
	got := parseDiskStatsFixture(t, "./test_files/diskstats_14field.txt")

	sda, ok := got["sda"]
	if !ok {
		t.Fatalf("sda missing, got devices %v", got)
	}
	if sda.Major != 8 || sda.Minor != 0 {
		t.Errorf("major/minor = %d/%d, want 8/0", sda.Major, sda.Minor)
	}
	if sda.ReadsCompleted != 145238 {
		t.Errorf("ReadsCompleted = %d, want 145238", sda.ReadsCompleted)
	}
	if sda.ReadsMerged != 12043 {
		t.Errorf("ReadsMerged = %d, want 12043", sda.ReadsMerged)
	}
	// field 6 is sectors, exposed as bytes
	if sda.BytesRead != 9834512*512 {
		t.Errorf("BytesRead = %d, want %d", sda.BytesRead, 9834512*512)
	}
	if sda.ReadTimeMs != 78213 {
		t.Errorf("ReadTimeMs = %d, want 78213", sda.ReadTimeMs)
	}
	if sda.WritesCompleted != 98234 {
		t.Errorf("WritesCompleted = %d, want 98234", sda.WritesCompleted)
	}
	if sda.BytesWritten != 4521888*512 {
		t.Errorf("BytesWritten = %d, want %d", sda.BytesWritten, 4521888*512)
	}
	if sda.IOTimeMs != 89231 {
		t.Errorf("IOTimeMs = %d, want 89231", sda.IOTimeMs)
	}
	if sda.WeightedIOTimeMs != 190257 {
		t.Errorf("WeightedIOTimeMs = %d, want 190257", sda.WeightedIOTimeMs)
	}

	// IOInProgress is the one gauge - sda2 has 2 in flight.
	if got["sda2"].IOInProgress != 2 {
		t.Errorf("sda2 IOInProgress = %d, want 2", got["sda2"].IOInProgress)
	}

	// Partitions are kept deliberately so callers can join to GetDisks.
	if _, ok := got["sda1"]; !ok {
		t.Errorf("sda1 should be kept - partitions join to GetDisks by name")
	}
}

func TestParseDiskStatsCapabilityFlags(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		wantDiscard bool
		wantFlush   bool
	}{
		{"14 field", "./test_files/diskstats_14field.txt", false, false},
		{"18 field", "./test_files/diskstats_18field.txt", true, false},
		{"20 field", "./test_files/diskstats_20field.txt", true, true},
		{"legacy 7 field", "./test_files/diskstats_legacy7field.txt", false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for device, d := range parseDiskStatsFixture(t, c.path) {
				if d.HasDiscardStats != c.wantDiscard {
					t.Errorf("%s HasDiscardStats = %v, want %v", device, d.HasDiscardStats, c.wantDiscard)
				}
				if d.HasFlushStats != c.wantFlush {
					t.Errorf("%s HasFlushStats = %v, want %v", device, d.HasFlushStats, c.wantFlush)
				}
			}
		})
	}
}

func TestParseDiskStatsDiscardAndFlush(t *testing.T) {
	got := parseDiskStatsFixture(t, "./test_files/diskstats_20field.txt")
	sda := got["sda"]

	if sda.DiscardsCompleted != 200 {
		t.Errorf("DiscardsCompleted = %d, want 200", sda.DiscardsCompleted)
	}
	if sda.BytesDiscarded != 16384*512 {
		t.Errorf("BytesDiscarded = %d, want %d", sda.BytesDiscarded, 16384*512)
	}
	if sda.DiscardTimeMs != 150 {
		t.Errorf("DiscardTimeMs = %d, want 150", sda.DiscardTimeMs)
	}
	if sda.FlushesCompleted != 900 {
		t.Errorf("FlushesCompleted = %d, want 900", sda.FlushesCompleted)
	}
	if sda.FlushTimeMs != 450 {
		t.Errorf("FlushTimeMs = %d, want 450", sda.FlushTimeMs)
	}
}

func TestParseDiskStatsLegacy7Field(t *testing.T) {
	got := parseDiskStatsFixture(t, "./test_files/diskstats_legacy7field.txt")
	hda := got["hda"]

	if hda.ReadsCompleted != 12345 {
		t.Errorf("ReadsCompleted = %d, want 12345", hda.ReadsCompleted)
	}
	if hda.BytesRead != 987654*512 {
		t.Errorf("BytesRead = %d, want %d", hda.BytesRead, 987654*512)
	}
	if hda.WritesCompleted != 4321 {
		t.Errorf("WritesCompleted = %d, want 4321", hda.WritesCompleted)
	}
	if hda.BytesWritten != 123456*512 {
		t.Errorf("BytesWritten = %d, want %d", hda.BytesWritten, 123456*512)
	}
	// Nothing past the 7th token exists in this form.
	if hda.IOTimeMs != 0 || hda.WeightedIOTimeMs != 0 {
		t.Errorf("expected no timing data in the legacy form, got %+v", hda)
	}
}

func TestIsIgnoredDiskDevice(t *testing.T) {
	ignored := []string{"loop0", "loop12", "ram0", "zram0", "fd0", "sr0"}
	kept := []string{"sda", "sda1", "nvme0n1", "nvme0n1p1", "dm-0", "md0", "vda", "xvda1", "loopback"}

	for _, name := range ignored {
		if !isIgnoredDiskDevice(name) {
			t.Errorf("%q should be ignored", name)
		}
	}
	for _, name := range kept {
		if isIgnoredDiskDevice(name) {
			t.Errorf("%q should be kept", name)
		}
	}
}

func TestParseDiskStatsFiltersPseudoDevices(t *testing.T) {
	for _, path := range []string{
		"./test_files/diskstats_14field.txt",
		"./test_files/diskstats_18field.txt",
		"./test_files/diskstats_20field.txt",
	} {
		for device := range parseDiskStatsFixture(t, path) {
			if isIgnoredDiskDevice(device) {
				t.Errorf("%s: %q leaked through the filter", path, device)
			}
		}
	}
}

func TestDiskIORatesSince(t *testing.T) {
	prev := DiskIO{Device: "sda", ReadsCompleted: 100, WritesCompleted: 50, BytesRead: 1024, BytesWritten: 2048, IOTimeMs: 1000}
	cur := DiskIO{Device: "sda", ReadsCompleted: 200, WritesCompleted: 70, BytesRead: 11264, BytesWritten: 12288, IOTimeMs: 1500}

	got := cur.RatesSince(prev, 2.0)

	if got.ReadsPerSec != 50 {
		t.Errorf("ReadsPerSec = %v, want 50", got.ReadsPerSec)
	}
	if got.WritesPerSec != 10 {
		t.Errorf("WritesPerSec = %v, want 10", got.WritesPerSec)
	}
	if got.ReadBytesPerSec != 5120 {
		t.Errorf("ReadBytesPerSec = %v, want 5120", got.ReadBytesPerSec)
	}
	if got.WriteBytesPerSec != 5120 {
		t.Errorf("WriteBytesPerSec = %v, want 5120", got.WriteBytesPerSec)
	}
	// 500ms of I/O time over a 2s window = 25% utilization.
	if got.UtilPercent != 25 {
		t.Errorf("UtilPercent = %v, want 25", got.UtilPercent)
	}
	if got.Device != "sda" {
		t.Errorf("Device = %q, want sda", got.Device)
	}
}

func TestDiskIORatesSinceGuards(t *testing.T) {
	prev := DiskIO{ReadsCompleted: 100, IOTimeMs: 1000}
	cur := DiskIO{ReadsCompleted: 200, IOTimeMs: 1500}

	if got := cur.RatesSince(prev, 0); got.ReadsPerSec != 0 {
		t.Errorf("zero elapsed should yield zeros, got %+v", got)
	}
	if got := cur.RatesSince(prev, -1); got.ReadsPerSec != 0 {
		t.Errorf("negative elapsed should yield zeros, got %+v", got)
	}
	// Counter reset (device removed and re-added) must not produce a
	// huge bogus rate from the uint64 underflow.
	if got := prev.RatesSince(cur, 2.0); got.ReadsPerSec != 0 {
		t.Errorf("counter reset should yield zeros, got %+v", got)
	}
}

func TestGetDiskIO(t *testing.T) {
	syStats := SyStats{DiskStatsPath: "./test_files/diskstats_20field.txt"}
	got, err := getDiskIO(&syStats)
	if err != nil {
		t.Fatalf("getDiskIO returned error: %s", err.Error())
	}

	// sda, sda1, md0 - zram0 and sr0 are filtered.
	if len(got) != 3 {
		t.Fatalf("got %d devices, want 3: %+v", len(got), got)
	}
	if got[0].Device != "sda" {
		t.Errorf("first device = %q, want sda (file order preserved)", got[0].Device)
	}
	for _, d := range got {
		if d.Time == 0 {
			t.Errorf("%s has no Time set", d.Device)
		}
	}
}

// TestGetDiskIORecoversFromPanic proves a malformed counter surfaces as a
// normal error rather than crashing, matching TestGetMemoryRecoversFromPanic.
func TestGetDiskIORecoversFromPanic(t *testing.T) {
	syStats := SyStats{DiskStatsPath: "./test_files/diskstats_corrupt.txt"}
	_, err := withRecover(func() ([]DiskIO, error) { return getDiskIO(&syStats) })
	if err == nil {
		t.Errorf("expected a recovered-panic error for a non-numeric counter")
	}
}

func TestGetDiskIOMissingFile(t *testing.T) {
	syStats := SyStats{DiskStatsPath: "./test_files/does_not_exist.txt"}
	if _, err := getDiskIO(&syStats); err == nil {
		t.Errorf("expected an error for a missing diskstats file")
	}
}
