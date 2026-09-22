package strops

import (
	"strconv"
)

// ToUint64 parses input, panicking on malformed content. Callers are
// reached through systats.withRecover, which turns the panic into a
// returned error - so a corrupt /proc line surfaces as an error rather
// than taking down the calling process.
func ToUint64(input string) uint64 {
	output, err := strconv.ParseUint(input, 10, 64)
	if err != nil {
		panic(err)
	}
	return output
}

// ToFloat64 parses input, panicking on malformed content. See ToUint64.
func ToFloat64(input string) float64 {
	output, err := strconv.ParseFloat(input, 64)
	if err != nil {
		panic(err)
	}
	return output
}
