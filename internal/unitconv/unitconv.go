package unitconv

func KibToKB(input uint64) uint64 {
	return uint64(float64(input) * 1.024)
}

func KibToMB(input uint64) uint64 {
	return KibToKB(input) / 1024
}

func KibToGB(input uint64) uint64 {
	return KibToMB(input) / 1024
}
