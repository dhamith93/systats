package systats

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
)

// Network holds interface information
type Network struct {
	Interface  string       `json:"interface"`
	Ip         string       `json:"ip"`
	Ipv6       string       `json:"ipv6"`
	MacAddress string       `json:"macAddress"`
	Usage      NetworkUsage `json:"usage"`
	Time       int64        `json:"time"`
}

// NetworkUsage holds Tx/Rx usage information
type NetworkUsage struct {
	State     string `json:"state"`
	RxBytes   uint64 `json:"rxBytes"`
	TxBytes   uint64 `json:"txBytes"`
	RxPackets uint64 `json:"rxPackets"`
	TxPackets uint64 `json:"txPackets"`
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

var httpClient = &http.Client{Timeout: 10 * time.Second}

func canConnect(url string) (bool, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return true, nil
}

func establishedTCPConnCount(process string) int {
	pids := findPidsByName(process)
	if len(pids) == 0 {
		return 0
	}

	establishedInodes := establishedTCPInodes()
	if len(establishedInodes) == 0 {
		return 0
	}

	count := 0
	for _, pid := range pids {
		count += countMatchingSocketFds(pid, establishedInodes)
	}
	return count
}

func findPidsByName(name string) []int {
	pids, err := listPids()
	if err != nil {
		return nil
	}

	matches := []int{}
	for _, pid := range pids {
		comm, err := fileops.ReadFileWithError("/proc/" + strconv.Itoa(pid) + "/comm")
		if err != nil {
			continue // process exited
		}
		if strings.TrimSpace(comm) == name {
			matches = append(matches, pid)
		}
	}
	return matches
}

func establishedTCPInodes() map[string]bool {
	inodes := map[string]bool{}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		content, err := fileops.ReadFileWithError(path)
		if err != nil {
			continue
		}

		lines := strings.Split(content, "\n")
		if len(lines) > 0 {
			lines = lines[1:] // skip header row
		}
		for _, line := range lines {
			// columns: sl local_address rem_address st tx_queue:rx_queue
			// tr:tm->when retrnsmt uid timeout inode ...
			fields := strings.Fields(line)
			if len(fields) < 10 {
				continue
			}
			if !strings.EqualFold(fields[3], "01") {
				continue // not ESTABLISHED
			}
			inodes[fields[9]] = true
		}
	}
	return inodes
}

func countMatchingSocketFds(pid int, establishedInodes map[string]bool) int {
	fdDir := "/proc/" + strconv.Itoa(pid) + "/fd"
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return 0 // process exited, or fds unreadable (permissions)
	}

	count := 0
	for _, e := range entries {
		target, err := os.Readlink(fdDir + "/" + e.Name())
		if err != nil {
			continue
		}
		if !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
			continue
		}
		inode := target[len("socket:[") : len(target)-1]
		if establishedInodes[inode] {
			count++
		}
	}
	return count
}
