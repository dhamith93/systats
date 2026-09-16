package systats

import (
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
	Interface  string
	Ip         string
	Ipv6       string
	MacAddress string
	Usage      NetworkUsage
	Time       int64
}

// NetworkUsage holds Tx/Rx usage information
type NetworkUsage struct {
	State     string
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
}

func getNetworks() ([]Network, error) {
	output := []Network{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return output, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return output, err
		}

		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		output = append(output, Network{
			Interface:  iface.Name,
			Ip:         pickIP(addrs, true),
			Ipv6:       pickIP(addrs, false),
			MacAddress: iface.HardwareAddr.String(),
			Usage: NetworkUsage{
				State:     readAsString("/sys/class/net/" + iface.Name + "/operstate"),
				RxBytes:   readAsUint64("/sys/class/net/" + iface.Name + "/statistics/rx_bytes"),
				TxBytes:   readAsUint64("/sys/class/net/" + iface.Name + "/statistics/tx_bytes"),
				RxPackets: readAsUint64("/sys/class/net/" + iface.Name + "/statistics/rx_packets"),
				TxPackets: readAsUint64("/sys/class/net/" + iface.Name + "/statistics/tx_packets"),
			},
			Time: time.Now().Unix(),
		})
	}

	return output, nil
}

func pickIP(addrs []net.Addr, wantV4 bool) string {
	fallback := ""
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if (ip.To4() != nil) != wantV4 {
			continue
		}
		if !ip.IsLinkLocalUnicast() {
			return ip.String()
		}
		if fallback == "" {
			fallback = ip.String()
		}
	}
	return fallback
}

func getNetworkUsage(networkInterface string) NetworkUsage {
	return NetworkUsage{
		State:     readAsString("/sys/class/net/" + networkInterface + "/operstate"),
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

func readAsString(path string) string {
	result, err := fileops.ReadFileWithError(path)
	if err != nil {
		return "error"
	}
	return strings.TrimSpace(result)
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
