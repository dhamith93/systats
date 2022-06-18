package systats

import (
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/exec"
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

func getDisks() ([]Disk, error) {
	output := []Disk{}

	dfOutput := getDiskInfo()
	dfOutputInode := getDiskInodeInfo()

	for i, disk := range dfOutput {
		diskArr := strings.Fields(disk)
		diskInodeArr := strings.Fields(dfOutputInode[i])

		if len(diskArr) < 6 || len(diskInodeArr) < 6 {
			continue
		}

		newDisk := Disk{}
		newDisk.FileSystem = diskArr[0]
		newDisk.Type = diskArr[1]
		newDisk.MountedOn = diskArr[6]
		newDisk.Usage = DiskUsage{}
		newDisk.Inodes = InodeUsage{}

		val, err := strconv.ParseUint(diskArr[2], 10, 64)
		if err != nil {
			return output, err
		}
		newDisk.Usage.Size = val
		val, err = strconv.ParseUint(diskArr[3], 10, 64)
		if err != nil {
			return output, err
		}
		newDisk.Usage.Used = val
		val, err = strconv.ParseUint(diskArr[4], 10, 64)
		if err != nil {
			return output, err
		}
		newDisk.Usage.Available = val
		newDisk.Usage.Usage = diskArr[5]
		newDisk.Usage.Unit = Byte

		val, err = strconv.ParseUint(diskInodeArr[2], 10, 64)
		if err != nil {
			return output, err
		}
		newDisk.Inodes.Inodes = val
		val, err = strconv.ParseUint(diskInodeArr[3], 10, 64)
		if err != nil {
			return output, err
		}
		newDisk.Inodes.Used = val
		val, err = strconv.ParseUint(diskInodeArr[4], 10, 64)
		if err != nil {
			return output, err
		}
		newDisk.Inodes.Available = uint64(val)
		newDisk.Inodes.Usage = diskInodeArr[5]
		newDisk.Time = time.Now().Unix()
		output = append(output, newDisk)
	}

	return output, nil
}

func getDiskInfo() []string {
	// Filesystem     Type  1K-blocks      Used Available Use% Mounted on
	result := exec.Execute("df", "-T", "-B1", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=udev")
	return strings.Split(result, "\n")[1:]
}

func getDiskInodeInfo() []string {
	// Filesystem     Type   Inodes   IUsed   IFree IUse% Mounted on
	result := exec.Execute("df", "-T", "-B1", "-i", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=udev")
	return strings.Split(result, "\n")[1:]
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
