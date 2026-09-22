package systats

import (
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"github.com/dhamith93/systats/internal/strops"
)

// Swap holds information on system swap usage.
//
// As with Memory, the size fields are float64 in the requested Unit so
// the larger units stay usable - see the Memory doc comment.
type Swap struct {
	PercentageUsed float64 `json:"percentageUsed"`
	Free           float64 `json:"free"`
	Used           float64 `json:"used"`
	Time           int64   `json:"time"`
	Total          float64 `json:"total"`
	Unit           string  `json:"unit"`
}

func getSwap(systats *SyStats, unit string) (Swap, error) {
	output := Swap{Unit: unit}

	// Resolved first so an unsupported unit fails before any file I/O.
	convert, err := kibConverter(unit)
	if err != nil {
		return output, err
	}

	meminfoStr, err := fileops.ReadFileWithError(systats.MeminfoPath)
	if err != nil {
		return output, err
	}

	totalKiB, freeKiB := parseSwapMeminfo(meminfoStr)
	var usedKiB uint64
	if totalKiB > 0 {
		usedKiB = totalKiB - freeKiB
		output.PercentageUsed = float64(usedKiB) / float64(totalKiB) * 100
	}

	output.Total = convert(totalKiB)
	output.Used = convert(usedKiB)
	output.Free = convert(freeKiB)
	output.Time = time.Now().Unix()

	return output, nil
}

// parseSwapMeminfo reads the swap figures (in KiB) from /proc/meminfo
// content.
func parseSwapMeminfo(content string) (totalKiB, freeKiB uint64) {
	for _, line := range strings.Split(content, "\n") {
		lineArr := strings.Fields(line)
		if len(lineArr) == 0 {
			continue
		}
		switch lineArr[0] {
		case "SwapTotal:":
			totalKiB = strops.ToUint64(lineArr[1])
		case "SwapFree:":
			freeKiB = strops.ToUint64(lineArr[1])
		}
	}
	return totalKiB, freeKiB
}
