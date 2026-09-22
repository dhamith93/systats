// Package unitconv converts the KiB-denominated values Linux reports in
// /proc/meminfo into the other units.
//
// All conversions here are binary (1024-based), matching what free(1),
// df(1) and top(1) display - those are the numbers a user will compare
// these against. Note /proc/meminfo labels its values "kB" but they are
// already KiB, so KibToKB is an identity conversion.
//
// Results are float64 because the larger units can't be represented
// usefully as integers: 4 GiB of RAM truncates to "3 GB", discarding a
// fifth of the value. Inputs are exact in float64 well past any
// plausible memory size (2^53 bytes is 8 PiB).
package unitconv

// KibToBytes converts KiB to bytes.
func KibToBytes(input uint64) float64 {
	return float64(input) * 1024
}

// KibToKB returns input unchanged: /proc/meminfo's "kB" values are
// already KiB, so there is nothing to convert.
func KibToKB(input uint64) float64 {
	return float64(input)
}

// KibToMB converts KiB to MiB.
func KibToMB(input uint64) float64 {
	return float64(input) / 1024
}

// KibToGB converts KiB to GiB.
func KibToGB(input uint64) float64 {
	return float64(input) / 1024 / 1024
}
