package systats

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"golang.org/x/sys/unix"
)

// Disk holds information on single disk
type Disk struct {
	FileSystem string
	Type       string
	MountedOn  string
	Usage      DiskUsage
	Inodes     InodeUsage
	Time       int64
}

// DiskUsage holds information on single disk usage information
type DiskUsage struct {
	Size      uint64
	Used      uint64
	Available uint64
	Usage     string
	Unit      string
}

// InodeUsage holds information on single disk inode usage
type InodeUsage struct {
	Inodes    uint64
	Available uint64
	Used      uint64
	Usage     string
}

type mountEntry struct {
	device     string
	mountPoint string
	fsType     string
}

// excludedFsTypes mirrors the --exclude-type filters the old `df`-based
// implementation used for pseudo filesystems that (unlike sysfs/proc/
// cgroup, which self-exclude below via a zero block count) report a
// real, non-zero size.
var excludedFsTypes = map[string]bool{
	"tmpfs":    true,
	"devtmpfs": true,
	"udev":     true,
}

func getDisks(systats *SyStats) ([]Disk, error) {
	mounts, err := readMounts(systats.MountsPath)
	if err != nil {
		return nil, err
	}

	output := []Disk{}
	for _, m := range mounts {
		if excludedFsTypes[m.fsType] {
			continue
		}

		var stat unix.Statfs_t
		if err := unix.Statfs(m.mountPoint, &stat); err != nil {
			continue // mount point disappeared, or isn't statable
		}

		// df itself never shows filesystems reporting zero blocks - this
		// is what filters out sysfs/proc/cgroup/etc without needing them
		// in excludedFsTypes above.
		if stat.Blocks == 0 {
			continue
		}

		blockSize := uint64(stat.Bsize)
		size := uint64(stat.Blocks) * blockSize
		free := uint64(stat.Bfree) * blockSize
		available := uint64(stat.Bavail) * blockSize
		used := size - free

		inodesTotal := uint64(stat.Files)
		inodesFree := uint64(stat.Ffree)
		inodesUsed := inodesTotal - inodesFree

		output = append(output, Disk{
			FileSystem: m.device,
			Type:       m.fsType,
			MountedOn:  m.mountPoint,
			Usage: DiskUsage{
				Size:      size,
				Used:      used,
				Available: available,
				Usage:     usagePercent(used, available),
				Unit:      Byte,
			},
			Inodes: InodeUsage{
				Inodes:    inodesTotal,
				Used:      inodesUsed,
				Available: inodesFree,
				Usage:     usagePercent(inodesUsed, inodesFree),
			},
			Time: time.Now().Unix(),
		})
	}

	return output, nil
}

// readMounts parses a /proc/mounts-formatted file into mountEntry values.
func readMounts(path string) ([]mountEntry, error) {
	content, err := fileops.ReadFileWithError(path)
	if err != nil {
		return nil, err
	}
	return parseMounts(content), nil
}

// parseMounts is split out from readMounts so the parsing/escaping logic
// can be unit-tested against fixture text without needing a real
// /proc/mounts file.
func parseMounts(content string) []mountEntry {
	entries := []mountEntry{}
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		entries = append(entries, mountEntry{
			device:     unescapeMountField(fields[0]),
			mountPoint: unescapeMountField(fields[1]),
			fsType:     fields[2],
		})
	}
	return entries
}

var mountFieldEscapes = strings.NewReplacer(
	`\040`, " ",
	`\011`, "\t",
	`\012`, "\n",
	`\134`, `\`,
)

func unescapeMountField(field string) string {
	return mountFieldEscapes.Replace(field)
}

// usagePercent replicates df's Use%/IUse% formula: ceil(100 * used /
// (used + avail)), not used/size*100 - avail excludes blocks/inodes
// reserved for root, so it isn't the same as (size - used).
func usagePercent(used, avail uint64) string {
	denom := used + avail
	if denom == 0 {
		return "0%"
	}
	pct := int(math.Ceil(100 * float64(used) / float64(denom)))
	return strconv.Itoa(pct) + "%"
}

func (d *Disk) Convert(unit string) {
	if d.Usage.Unit == Byte {
		if unit == Kilobyte {
			d.Usage.Size = d.Usage.Size / 1024
			d.Usage.Used = d.Usage.Used / 1024
			d.Usage.Available = d.Usage.Available / 1024
		}
		if unit == Megabyte {
			d.Usage.Size = d.Usage.Size / 1024 / 1024
			d.Usage.Used = d.Usage.Used / 1024 / 1024
			d.Usage.Available = d.Usage.Available / 1024 / 1024
		}
		if unit == Gigabyte {
			d.Usage.Size = d.Usage.Size / 1024 / 1024 / 1024
			d.Usage.Used = d.Usage.Used / 1024 / 1024 / 1024
			d.Usage.Available = d.Usage.Available / 1024 / 1024 / 1024
		}
	}

	if d.Usage.Unit == Kilobyte {
		if unit == Byte {
			d.Usage.Size = d.Usage.Size * 1024
			d.Usage.Used = d.Usage.Used * 1024
			d.Usage.Available = d.Usage.Available * 1024
		}
		if unit == Megabyte {
			d.Usage.Size = d.Usage.Size / 1024
			d.Usage.Used = d.Usage.Used / 1024
			d.Usage.Available = d.Usage.Available / 1024
		}
		if unit == Gigabyte {
			d.Usage.Size = d.Usage.Size / 1024 / 1024
			d.Usage.Used = d.Usage.Used / 1024 / 1024
			d.Usage.Available = d.Usage.Available / 1024 / 1024
		}
	}

	if d.Usage.Unit == Megabyte {
		if unit == Byte {
			d.Usage.Size = d.Usage.Size * 1024 * 1024
			d.Usage.Used = d.Usage.Used * 1024 * 1024
			d.Usage.Available = d.Usage.Available * 1024 * 1024
		}
		if unit == Kilobyte {
			d.Usage.Size = d.Usage.Size * 1024
			d.Usage.Used = d.Usage.Used * 1024
			d.Usage.Available = d.Usage.Available * 1024
		}
		if unit == Gigabyte {
			d.Usage.Size = d.Usage.Size / 1024
			d.Usage.Used = d.Usage.Used / 1024
			d.Usage.Available = d.Usage.Available / 1024
		}
	}

	if d.Usage.Unit == Gigabyte {
		if unit == Byte {
			d.Usage.Size = d.Usage.Size * 1024 * 1024 * 1024
			d.Usage.Used = d.Usage.Used * 1024 * 1024 * 1024
			d.Usage.Available = d.Usage.Available * 1024 * 1024 * 1024
		}
		if unit == Kilobyte {
			d.Usage.Size = d.Usage.Size * 1024 * 1024
			d.Usage.Used = d.Usage.Used * 1024 * 1024
			d.Usage.Available = d.Usage.Available * 1024 * 1024
		}
		if unit == Megabyte {
			d.Usage.Size = d.Usage.Size * 1024
			d.Usage.Used = d.Usage.Used * 1024
			d.Usage.Available = d.Usage.Available * 1024
		}
	}

	d.Usage.Unit = unit
}
