package systats

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/exec"
	"github.com/dhamith93/systats/internal/fileops"
)

// Network holds interface information
type Network struct {
	Interface string
	Ip        string
	Usage     NetworkUsage
	Time      int64
}

// NetworkUsage holds Tx/Rx usage information
type NetworkUsage struct {
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
}

func getNetworks() ([]Network, error) {
	output := []Network{}
	ipCommand := exec.GetExecPath("ip")
	if ipCommand == "" {
		return output, errors.New("cannot find `ip` command path")
	}

	execCommand := ipCommand + " -o addr show scope global | awk '{split($4, a, \"/\"); print $2\" : \"a[1]}'"
	result := exec.ExecuteWithPipe(execCommand)
	resultSplit := strings.Split(result, "\n")

	for _, iface := range resultSplit {
		ifaceArray := strings.Fields(iface)
		if len(ifaceArray) != 3 {
			continue
		}
		output = append(output, Network{
			Interface: ifaceArray[0],
			Ip:        ifaceArray[2],
			Usage: NetworkUsage{
				RxBytes:   readAsUint64("/sys/class/net/" + ifaceArray[0] + "/statistics/rx_bytes"),
				TxBytes:   readAsUint64("/sys/class/net/" + ifaceArray[0] + "/statistics/tx_bytes"),
				RxPackets: readAsUint64("/sys/class/net/" + ifaceArray[0] + "/statistics/rx_packets"),
				TxPackets: readAsUint64("/sys/class/net/" + ifaceArray[0] + "/statistics/tx_packets"),
			},
			Time: time.Now().Unix(),
		})
	}

	return output, nil
}

func getNetworkUsage(networkInterface string) NetworkUsage {
	return NetworkUsage{
		RxBytes:   readAsUint64("/sys/class/net/" + networkInterface + "/statistics/rx_bytes"),
		TxBytes:   readAsUint64("/sys/class/net/" + networkInterface + "/statistics/tx_bytes"),
		RxPackets: readAsUint64("/sys/class/net/" + networkInterface + "/statistics/rx_packets"),
		TxPackets: readAsUint64("/sys/class/net/" + networkInterface + "/statistics/tx_packets"),
	}
}

func readAsUint64(path string) uint64 {
	result, err := fileops.ReadFileWithError(path)
	if err != nil {
		return 0
	}
	out, _ := strconv.ParseUint(strings.TrimSpace(result), 10, 64)
	return out
}

func isPortOpen(port int) bool {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func canConnect(url string) (bool, error) {
	status := false
	resp, err := http.Get(url)
	if err == nil {
		status = true
	}
	defer resp.Body.Close()
	return status, err
}

func establishedTCPConnCount(process string) int {
	count := 0
	command := "lsof -ni | grep ESTABLISHED"
	resArr := strings.Split(exec.ExecuteWithPipe(command), "\n")
	for _, line := range resArr {
		lineArr := strings.Fields(line)
		if len(lineArr) > 0 && strings.TrimSpace(lineArr[0]) == process {
			count += 1
		}
	}
	return count
}
