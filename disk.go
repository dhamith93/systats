package systats

import (
	"strconv"
	"strings"
)

// Disk holds information on single disk
type Disk struct {
	FileSystem string
	Type       string
	MountedOn  string
	Usage      DiskUsage
	Inodes     InodeUsage
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

		val, err := strconv.Atoi(diskArr[2])
		if err != nil {
			return output, err
		}
		newDisk.Usage.Size = uint64(val)
		val, err = strconv.Atoi(diskArr[3])
		if err != nil {
			return output, err
		}
		newDisk.Usage.Used = uint64(val)
		val, err = strconv.Atoi(diskArr[4])
		if err != nil {
			return output, err
		}
		newDisk.Usage.Available = uint64(val)
		newDisk.Usage.Usage = diskArr[5]
		newDisk.Usage.Unit = Byte

		val, err = strconv.Atoi(diskInodeArr[3])
		if err != nil {
			return output, err
		}
		newDisk.Inodes.Inodes = uint64(val)
		val, err = strconv.Atoi(diskInodeArr[3])
		if err != nil {
			return output, err
		}
		newDisk.Inodes.Used = uint64(val)
		val, err = strconv.Atoi(diskInodeArr[4])
		if err != nil {
			return output, err
		}
		newDisk.Inodes.Available = uint64(val)
		newDisk.Inodes.Usage = diskInodeArr[5]
		output = append(output, newDisk)
	}

	return output, nil
}

func getDiskInfo() []string {
	// Filesystem     Type  1K-blocks      Used Available Use% Mounted on
	result := Execute("df", "-T", "-B1", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=udev")
	return strings.Split(result, "\n")[1:]
}

func getDiskInodeInfo() []string {
	// Filesystem     Type   Inodes   IUsed   IFree IUse% Mounted on
	result := Execute("df", "-T", "-B1", "-i", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=udev")
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
