package strops

import (
	"strconv"

	"github.com/dhamith93/systats/internal/logger"
)

func ToUint64(input string) uint64 {
	output, err := strconv.ParseUint(input, 10, 64)
	if err != nil {
		logger.Log("error", "cannot parse "+input+" as unit64")
		panic(err)
	}
	return output
}

func ToFloat64(input string) float64 {
	output, err := strconv.ParseFloat(input, 64)
	if err != nil {
		logger.Log("error", "cannot parse "+input+" as float64")
		panic(err)
	}
	return output
}
