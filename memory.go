package systats

import (
	"errors"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"github.com/dhamith93/systats/internal/strops"
	"github.com/dhamith93/systats/internal/unitconv"
)

// Memory holds information on system memory usage
type Memory struct {
	PercentageUsed float64 `json:"percentageUsed"`
	Available      uint64  `json:"available"`
	Free           uint64  `json:"free"`
	Used           uint64  `json:"used"`
	Time           int64   `json:"time"`
	Total          uint64  `json:"total"`
	Unit           string  `json:"unit"`
}

func getMemory(systats *SyStats, unit string) (Memory, error) {
	output := Memory{}
	output.Unit = unit

	meminfoStr, err := fileops.ReadFileWithError(systats.MeminfoPath)
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
		percentage := float64(output.Used) / float64(output.Total) * 100
		output.PercentageUsed = percentage
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
