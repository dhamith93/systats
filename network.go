package systats

import (
	"errors"
	"strconv"
	"strings"
)

type Network struct {
	Interface string
	Ip        string
	RxBytes   uint64
	TxBytes   uint64
}

func getNetworks(systats *SyStats) ([]Network, error) {
	output := []Network{}
	ipCommand := GetExecPath("ip")
	if ipCommand == "" {
		return output, errors.New("cannot find `ip` command path")
	}

	execCommand := ipCommand + " -o addr show scope global | awk '{split($4, a, \"/\"); print $2\" : \"a[1]}'"
	result := ExecuteWithPipe(execCommand)
	resultSplit := strings.Split(result, "\n")

	for _, iface := range resultSplit {
		ifaceArray := strings.Fields(iface)
		if len(ifaceArray) != 3 {
			continue
		}
		output = append(output, Network{
			Interface: ifaceArray[0],
			Ip:        ifaceArray[2],
			RxBytes:   getBytes("/sys/class/net/" + ifaceArray[0] + "/statistics/rx_bytes"),
			TxBytes:   getBytes("/sys/class/net/" + ifaceArray[0] + "/statistics/tx_bytes"),
		})
	}

	return output, nil
}

func getBytes(path string) uint64 {
	result, err := readFile(path)
	if err != nil {
		return 0
	}
	out, _ := strconv.ParseUint(strings.TrimSpace(result), 10, 64)
	return out
}
