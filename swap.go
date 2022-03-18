package systats

import (
	"errors"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"github.com/dhamith93/systats/internal/strops"
	"github.com/dhamith93/systats/internal/unitconv"
)

// Swap holds information on system swap usage
type Swap struct {
	PercentageUsed float64
	Free           uint64
	Used           uint64
	Time           int64
	Total          uint64
	Unit           string
}

func getSwap(systats *SyStats, unit string) (Swap, error) {
	output := Swap{}
	output.Unit = unit

	meminfoStr, err := fileops.ReadFileWithError(systats.MeminfoPath)
	if err != nil {
		return output, err
	}

	meminfoSplit := strings.Split(meminfoStr, "\n")

	for _, line := range meminfoSplit {
		lineArr := strings.Fields(line)
		if len(lineArr) == 0 {
			continue
		}
		if lineArr[0] == "SwapTotal:" {
			output.Total = strops.ToUint64(lineArr[1])
		}
		if lineArr[0] == "SwapFree:" {
			output.Free = strops.ToUint64(lineArr[1])
		}
	}

	if output.Total > 0 {
		output.Used = output.Total - output.Free
		percentage := float64(output.Used) / float64(output.Total) * 100
		output.PercentageUsed = percentage
	}

	output.Time = time.Now().Unix()

	if unit == Kilobyte {
		output.Total = unitconv.KibToKB(output.Total)
		output.Used = unitconv.KibToKB(output.Used)
		output.Free = unitconv.KibToKB(output.Free)
	} else if unit == Megabyte {
		output.Total = unitconv.KibToMB(output.Total)
		output.Used = unitconv.KibToMB(output.Used)
		output.Free = unitconv.KibToMB(output.Free)
	} else {
		return output, errors.New(unit + " is not supported")
	}

	return output, nil
}
