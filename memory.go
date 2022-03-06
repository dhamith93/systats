package systats

import (
	"errors"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"github.com/dhamith93/systats/internal/strops"
	"github.com/dhamith93/systats/internal/unitconv"
)

type Memory struct {
	PrecentageUsed float64
	Available      uint64
	Free           uint64
	Used           uint64
	Time           int64
	Total          uint64
	Unit           string
}

func readMeminfoFile(path string) (string, error) {
	if !fileops.IsFile(path) {
		return "", errors.New(path + " file not found")
	}

	return fileops.ReadFile(path), nil
}

func getMemory(systats *SyStats, unit string) (Memory, error) {
	output := Memory{}
	output.Unit = unit

	meminfoStr, err := readMeminfoFile(systats.MeminfoPath)
	if err != nil {
		return output, err
	}

	meminfoSplit := strings.Split(meminfoStr, "\n")
	var buffers, cached uint64

	for _, line := range meminfoSplit {
		lineArr := strings.Fields(line)
		if len(lineArr) == 0 {
			continue
		}
		if lineArr[0] == "MemTotal:" {
			output.Total = strops.ToUint64(lineArr[1])
		}
		if lineArr[0] == "MemFree:" {
			output.Free = strops.ToUint64(lineArr[1])
		}
		if lineArr[0] == "MemAvailable:" {
			output.Available = strops.ToUint64(lineArr[1])
		}
		if lineArr[0] == "Buffers:" {
			buffers = strops.ToUint64(lineArr[1])
		}
		if lineArr[0] == "Cached:" {
			cached = strops.ToUint64(lineArr[1])
		}
	}

	if output.Total > 0 {
		output.Used = output.Total - (output.Free + buffers + cached)
		precentage := float64(output.Used) / float64(output.Total) * 100
		output.PrecentageUsed = precentage
	}

	output.Time = time.Now().Unix()

	if unit == Kilobyte {
		output.Available = unitconv.KibToKB(output.Available)
		output.Total = unitconv.KibToKB(output.Total)
		output.Used = unitconv.KibToKB(output.Used)
		output.Free = unitconv.KibToKB(output.Free)
	} else if unit == Megabyte {
		output.Available = unitconv.KibToMB(output.Available)
		output.Total = unitconv.KibToMB(output.Total)
		output.Used = unitconv.KibToMB(output.Used)
		output.Free = unitconv.KibToMB(output.Free)
	} else {
		return output, errors.New(unit + " is not supported")
	}

	return output, nil
}