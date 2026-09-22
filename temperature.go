package systats

import (
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Temperature is one temperature sensor reading from /sys/class/hwmon.
type Temperature struct {
	// Name is the hwmon chip name (coretemp, k10temp, cpu_thermal, ...).
	Name string `json:"name"`
	// Label is the per-sensor label ("Package id 0", "Core 0") when the
	// chip publishes one, else the sensor's file prefix ("temp1").
	Label   string  `json:"label"`
	Celsius float64 `json:"celsius"`
	// High and Critical are the chip's own thresholds. Not every sensor
	// publishes them - check HighAvailable/CriticalAvailable before
	// treating a zero as a real threshold, or a reading will look
	// permanently over-limit.
	High              float64 `json:"high"`
	HighAvailable     bool    `json:"highAvailable"`
	Critical          float64 `json:"critical"`
	CriticalAvailable bool    `json:"criticalAvailable"`
}

// getTemperatures walks /sys/class/hwmon/hwmon*/temp*_input. Values there
// are millidegrees Celsius.
//
// An unreadable sensor is skipped rather than failing the batch: hwmon
// entries disappear when a module unloads, and some publish an _input
// file that returns EIO until the chip is polled.
func getTemperatures(systats *SyStats) ([]Temperature, error) {
	output := []Temperature{}

	chips, err := os.ReadDir(systats.HwmonPath)
	if err != nil {
		// No hwmon at all: a VM or container usually has none. Normal
		// condition, so an empty slice rather than an error.
		return output, nil
	}

	for _, chip := range chips {
		chipDir := path.Join(systats.HwmonPath, chip.Name())
		chipName := readAsString(path.Join(chipDir, "name"))

		entries, err := os.ReadDir(chipDir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			prefix, ok := tempInputPrefix(e.Name())
			if !ok {
				continue
			}

			raw := readAsString(path.Join(chipDir, e.Name()))
			milli, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				continue // sensor present but not readable right now
			}

			t := Temperature{
				Name:    chipName,
				Label:   prefix,
				Celsius: milli / 1000,
			}
			if label := readAsString(path.Join(chipDir, prefix+"_label")); label != "" {
				t.Label = label
			}
			t.High, t.HighAvailable = readMilliDegrees(chipDir, prefix+"_max")
			t.Critical, t.CriticalAvailable = readMilliDegrees(chipDir, prefix+"_crit")

			output = append(output, t)
		}
	}

	// os.ReadDir sorts per directory, but readings accumulate across
	// chips - sort the whole set so output is stable between calls.
	sort.Slice(output, func(i, j int) bool {
		if output[i].Name != output[j].Name {
			return output[i].Name < output[j].Name
		}
		return output[i].Label < output[j].Label
	})

	return output, nil
}

// tempInputPrefix matches "tempN_input" and returns the "tempN" prefix
// that its _label/_max/_crit siblings share.
func tempInputPrefix(fileName string) (prefix string, ok bool) {
	if !strings.HasPrefix(fileName, "temp") || !strings.HasSuffix(fileName, "_input") {
		return "", false
	}
	prefix = strings.TrimSuffix(fileName, "_input")
	// Guard against a hypothetical "temp_input" with no index.
	if prefix == "temp" {
		return "", false
	}
	return prefix, true
}

func readMilliDegrees(dir, fileName string) (value float64, ok bool) {
	raw := readAsString(path.Join(dir, fileName))
	if raw == "" {
		return 0, false
	}
	milli, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return milli / 1000, true
}
