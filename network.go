package systats

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
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
		return ""
	}
	return strings.TrimSpace(result)
}

var dialer = &net.Dialer{}

func isPortOpen(ctx context.Context, port int) bool {
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func canConnect(ctx context.Context, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return true, nil
}

func establishedTCPConnCount(systats *SyStats, process string) int {
	pids := findPidsByName(systats.ProcPath, process)
	if len(pids) == 0 {
		return 0
	}

	establishedInodes := establishedTCPInodes(systats)
	if len(establishedInodes) == 0 {
		return 0
	}

	count := 0
	for _, pid := range pids {
		count += countMatchingSocketFds(systats.ProcPath, pid, establishedInodes)
	}
	return count
}

func findPidsByName(procPath string, name string) []int {
	pids, err := listPids(procPath)
	if err != nil {
		return nil
	}

	matches := []int{}
	for _, pid := range pids {
		comm, err := fileops.ReadFileWithError(procFilePath(procPath, pid, "comm"))
		if err != nil {
			continue // process exited
		}
		if strings.TrimSpace(comm) == name {
			matches = append(matches, pid)
		}
	}
	return matches
}

// tcpSocketRecord is one socket line from /proc/net/tcp or
// /proc/net/tcp6.
type tcpSocketRecord struct {
	state string // raw hex from the "st" column, e.g. "01"
	inode string
}

// parseProcNetTCP parses /proc/net/tcp or /proc/net/tcp6. Columns are:
// sl local_address rem_address st tx_queue:rx_queue tr:tm->when retrnsmt
// uid timeout inode ...
func parseProcNetTCP(content string) []tcpSocketRecord {
	out := []tcpSocketRecord{}

	lines := strings.Split(content, "\n")
	if len(lines) > 0 {
		lines = lines[1:] // skip header row
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		out = append(out, tcpSocketRecord{state: fields[3], inode: fields[9]})
	}

	return out
}

// readTCPSocketRecords reads both the IPv4 and IPv6 socket tables. A
// missing file is skipped rather than fatal - an IPv6-disabled host has
// no /proc/net/tcp6 at all.
func readTCPSocketRecords(systats *SyStats) []tcpSocketRecord {
	records := []tcpSocketRecord{}
	for _, path := range []string{systats.NetTCPPath, systats.NetTCP6Path} {
		content, err := fileops.ReadFileWithError(path)
		if err != nil {
			continue
		}
		records = append(records, parseProcNetTCP(content)...)
	}
	return records
}

func establishedTCPInodes(systats *SyStats) map[string]bool {
	inodes := map[string]bool{}
	for _, r := range readTCPSocketRecords(systats) {
		if strings.EqualFold(r.state, tcpStateEstablished) {
			inodes[r.inode] = true
		}
	}
	return inodes
}

func countMatchingSocketFds(procPath string, pid int, establishedInodes map[string]bool) int {
	fdDir := path.Join(procPath, strconv.Itoa(pid), "fd")
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

// TCP connection states as they appear in the "st" column of
// /proc/net/tcp, per include/net/tcp_states.h.
const (
	tcpStateEstablished = "01"
	tcpStateSynSent     = "02"
	tcpStateSynRecv     = "03"
	tcpStateFinWait1    = "04"
	tcpStateFinWait2    = "05"
	tcpStateTimeWait    = "06"
	tcpStateClose       = "07"
	tcpStateCloseWait   = "08"
	tcpStateLastAck     = "09"
	tcpStateListen      = "0A"
	tcpStateClosing     = "0B"
	tcpStateNewSynRecv  = "0C"
)

// TCPStates counts TCP sockets by connection state, combining IPv4
// (/proc/net/tcp) and IPv6 (/proc/net/tcp6).
//
// These are counts for every socket in the caller's network namespace,
// not just the calling process's - EstablishedTCPConnCount is the
// per-process equivalent.
type TCPStates struct {
	Established int `json:"established"`
	SynSent     int `json:"synSent"`
	SynRecv     int `json:"synRecv"`
	FinWait1    int `json:"finWait1"`
	FinWait2    int `json:"finWait2"`
	TimeWait    int `json:"timeWait"`
	Close       int `json:"close"`
	CloseWait   int `json:"closeWait"`
	LastAck     int `json:"lastAck"`
	Listen      int `json:"listen"`
	Closing     int `json:"closing"`
	NewSynRecv  int `json:"newSynRecv"`
	// Unknown counts sockets whose state byte isn't one of the above.
	// Non-zero means the kernel grew a state this library doesn't know
	// yet - Total still reconciles either way.
	Unknown int   `json:"unknown"`
	Total   int   `json:"total"`
	Time    int64 `json:"time"`
}

func (t *TCPStates) add(stateHex string) {
	t.Total++
	switch strings.ToUpper(stateHex) {
	case tcpStateEstablished:
		t.Established++
	case tcpStateSynSent:
		t.SynSent++
	case tcpStateSynRecv:
		t.SynRecv++
	case tcpStateFinWait1:
		t.FinWait1++
	case tcpStateFinWait2:
		t.FinWait2++
	case tcpStateTimeWait:
		t.TimeWait++
	case tcpStateClose:
		t.Close++
	case tcpStateCloseWait:
		t.CloseWait++
	case tcpStateLastAck:
		t.LastAck++
	case tcpStateListen:
		t.Listen++
	case tcpStateClosing:
		t.Closing++
	case tcpStateNewSynRecv:
		t.NewSynRecv++
	default:
		t.Unknown++
	}
}

// tcpStateName maps a /proc/net/tcp state byte to its kernel name, or
// "UNKNOWN" for anything not in the table.
func tcpStateName(stateHex string) string {
	switch strings.ToUpper(stateHex) {
	case tcpStateEstablished:
		return "ESTABLISHED"
	case tcpStateSynSent:
		return "SYN_SENT"
	case tcpStateSynRecv:
		return "SYN_RECV"
	case tcpStateFinWait1:
		return "FIN_WAIT1"
	case tcpStateFinWait2:
		return "FIN_WAIT2"
	case tcpStateTimeWait:
		return "TIME_WAIT"
	case tcpStateClose:
		return "CLOSE"
	case tcpStateCloseWait:
		return "CLOSE_WAIT"
	case tcpStateLastAck:
		return "LAST_ACK"
	case tcpStateListen:
		return "LISTEN"
	case tcpStateClosing:
		return "CLOSING"
	case tcpStateNewSynRecv:
		return "NEW_SYN_RECV"
	default:
		return "UNKNOWN"
	}
}

func getTCPConnectionStates(systats *SyStats) (TCPStates, error) {
	states := TCPStates{Time: time.Now().Unix()}
	for _, r := range readTCPSocketRecords(systats) {
		states.add(r.state)
	}
	return states, nil
}
