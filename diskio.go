package systats

import (
	"regexp"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"github.com/dhamith93/systats/internal/strops"
)

// diskSectorSizeBytes is fixed at 512 deliberately. The kernel normalizes
// /proc/diskstats sector counts to 512-byte units regardless of the
// device's logical or physical block size, so a 4Kn drive still reports
// 512-byte sectors here. Reading /sys/block/<dev>/queue/hw_sector_size
// and multiplying by it inflates byte counts 8x on those drives.
const diskSectorSizeBytes = 512

// DiskIO holds cumulative I/O counters for one block device, as reported
// by /proc/diskstats. Every field except IOInProgress is a monotonic
// counter since boot - use RatesSince to turn two samples into rates.
type DiskIO struct {
	Device string `json:"device"`
	Major  uint64 `json:"major"`
	Minor  uint64 `json:"minor"`

	ReadsCompleted uint64 `json:"readsCompleted"`
	ReadsMerged    uint64 `json:"readsMerged"`
	BytesRead      uint64 `json:"bytesRead"`
	ReadTimeMs     uint64 `json:"readTimeMs"`

	WritesCompleted uint64 `json:"writesCompleted"`
	WritesMerged    uint64 `json:"writesMerged"`
	BytesWritten    uint64 `json:"bytesWritten"`
	WriteTimeMs     uint64 `json:"writeTimeMs"`

	// IOInProgress is the only gauge here - I/Os currently queued or in
	// flight, not a running total.
	IOInProgress     uint64 `json:"ioInProgress"`
	IOTimeMs         uint64 `json:"ioTimeMs"`
	WeightedIOTimeMs uint64 `json:"weightedIoTimeMs"`

	// Discard stats exist on kernel 4.18+; check HasDiscardStats before
	// treating zeros as real.
	DiscardsCompleted uint64 `json:"discardsCompleted"`
	DiscardsMerged    uint64 `json:"discardsMerged"`
	BytesDiscarded    uint64 `json:"bytesDiscarded"`
	DiscardTimeMs     uint64 `json:"discardTimeMs"`
	HasDiscardStats   bool   `json:"hasDiscardStats"`

	// Flush stats exist on kernel 5.5+; check HasFlushStats likewise.
	FlushesCompleted uint64 `json:"flushesCompleted"`
	FlushTimeMs      uint64 `json:"flushTimeMs"`
	HasFlushStats    bool   `json:"hasFlushStats"`

	Time int64 `json:"time"`
}

// DiskIORates holds per-second rates derived from two DiskIO samples.
type DiskIORates struct {
	Device           string  `json:"device"`
	ReadsPerSec      float64 `json:"readsPerSec"`
	WritesPerSec     float64 `json:"writesPerSec"`
	ReadBytesPerSec  float64 `json:"readBytesPerSec"`
	WriteBytesPerSec float64 `json:"writeBytesPerSec"`
	// UtilPercent is the share of elapsed time the device spent servicing
	// I/O - the same figure iostat reports as %util.
	UtilPercent float64 `json:"utilPercent"`
}

// RatesSince computes per-second rates between prev and d, which must be
// samples of the same device. Returns zeros if elapsedSeconds is
// non-positive or if any counter went backwards (device removed and
// re-added, or samples passed in the wrong order).
func (d DiskIO) RatesSince(prev DiskIO, elapsedSeconds float64) DiskIORates {
	rates := DiskIORates{Device: d.Device}
	if elapsedSeconds <= 0 {
		return rates
	}
	if d.ReadsCompleted < prev.ReadsCompleted ||
		d.WritesCompleted < prev.WritesCompleted ||
		d.BytesRead < prev.BytesRead ||
		d.BytesWritten < prev.BytesWritten ||
		d.IOTimeMs < prev.IOTimeMs {
		return rates
	}

	rates.ReadsPerSec = float64(d.ReadsCompleted-prev.ReadsCompleted) / elapsedSeconds
	rates.WritesPerSec = float64(d.WritesCompleted-prev.WritesCompleted) / elapsedSeconds
	rates.ReadBytesPerSec = float64(d.BytesRead-prev.BytesRead) / elapsedSeconds
	rates.WriteBytesPerSec = float64(d.BytesWritten-prev.BytesWritten) / elapsedSeconds
	rates.UtilPercent = float64(d.IOTimeMs-prev.IOTimeMs) / (elapsedSeconds * 1000) * 100
	return rates
}

// ignoredDiskDevicePattern drops pseudo-devices that carry no useful I/O
// signal. Partitions are deliberately kept: GetDisks names devices as
// they appear in /proc/mounts ("/dev/sda2"), so keeping "sda2" here is
// what lets a caller join the two by
// path.Base(disk.FileSystem) == diskIO.Device.
var ignoredDiskDevicePattern = regexp.MustCompile(`^(loop|ram|zram|fd|sr)\d+$`)

func isIgnoredDiskDevice(name string) bool {
	return ignoredDiskDevicePattern.MatchString(name)
}

func getDiskIO(systats *SyStats) ([]DiskIO, error) {
	content, err := fileops.ReadFileWithError(systats.DiskStatsPath)
	if err != nil {
		return nil, err
	}
	return parseDiskStats(content), nil
}

// parseDiskStats parses /proc/diskstats. Line length varies by kernel: 14
// tokens (2.6.25+), 18 (4.18+, adds discard stats), 20 (5.5+, adds flush
// stats), plus a 7-token partition-only form on pre-2.6.25. Fields are
// read positionally only up to what the line actually contains, so a
// newer kernel appending further columns degrades to "extra data
// ignored" rather than breaking.
func parseDiskStats(content string) []DiskIO {
	out := []DiskIO{}
	now := time.Now().Unix()

	for _, line := range strings.Split(content, "\n") {
		f := strings.Fields(line)
		if len(f) < 7 {
			continue
		}
		name := f[2]
		if isIgnoredDiskDevice(name) {
			continue
		}

		d := DiskIO{
			Device: name,
			Major:  strops.ToUint64(f[0]),
			Minor:  strops.ToUint64(f[1]),
			Time:   now,
		}

		if len(f) < 14 {
			// Legacy 7-token partition form: reads, sectors read, writes,
			// sectors written. Nothing else is present.
			d.ReadsCompleted = strops.ToUint64(f[3])
			d.BytesRead = strops.ToUint64(f[4]) * diskSectorSizeBytes
			d.WritesCompleted = strops.ToUint64(f[5])
			d.BytesWritten = strops.ToUint64(f[6]) * diskSectorSizeBytes
			out = append(out, d)
			continue
		}

		d.ReadsCompleted = strops.ToUint64(f[3])
		d.ReadsMerged = strops.ToUint64(f[4])
		d.BytesRead = strops.ToUint64(f[5]) * diskSectorSizeBytes
		d.ReadTimeMs = strops.ToUint64(f[6])
		d.WritesCompleted = strops.ToUint64(f[7])
		d.WritesMerged = strops.ToUint64(f[8])
		d.BytesWritten = strops.ToUint64(f[9]) * diskSectorSizeBytes
		d.WriteTimeMs = strops.ToUint64(f[10])
		d.IOInProgress = strops.ToUint64(f[11])
		d.IOTimeMs = strops.ToUint64(f[12])
		d.WeightedIOTimeMs = strops.ToUint64(f[13])

		if len(f) >= 18 {
			d.HasDiscardStats = true
			d.DiscardsCompleted = strops.ToUint64(f[14])
			d.DiscardsMerged = strops.ToUint64(f[15])
			d.BytesDiscarded = strops.ToUint64(f[16]) * diskSectorSizeBytes
			d.DiscardTimeMs = strops.ToUint64(f[17])
		}
		if len(f) >= 20 {
			d.HasFlushStats = true
			d.FlushesCompleted = strops.ToUint64(f[18])
			d.FlushTimeMs = strops.ToUint64(f[19])
		}

		out = append(out, d)
	}

	return out
}
