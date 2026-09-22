package systats

import (
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
)

// ProtocolStats holds per-protocol network counters from /proc/net/snmp
// and /proc/net/netstat, keyed by protocol ("Ip", "Icmp", "IcmpMsg",
// "Tcp", "Udp", "UdpLite", "TcpExt", "IpExt") then by counter name
// exactly as the kernel spells it ("RetransSegs", "InErrs", "NoPorts").
//
// Deliberately untyped: the counter set varies by kernel version, and
// IcmpMsg's columns are InType<N>/OutType<N> for whatever ICMP types the
// host has actually seen, so they change over a machine's uptime. A
// fixed struct would report 0 for a counter the kernel never emitted,
// which is indistinguishable from a real 0 - use Value to tell them
// apart.
//
// /proc/net/snmp6 is not included: it uses a different one-name-per-line
// layout and would need its own parser.
type ProtocolStats struct {
	Protocols map[string]map[string]int64 `json:"protocols"`
	Time      int64                       `json:"time"`
}

// Value returns a single counter. ok is false when the protocol or the
// counter isn't present, which is the only way to distinguish "this
// kernel doesn't implement that counter" from "the counter is zero".
func (p ProtocolStats) Value(protocol, counter string) (int64, bool) {
	counters, ok := p.Protocols[protocol]
	if !ok {
		return 0, false
	}
	v, ok := counters[counter]
	return v, ok
}

func getProtocolStats(systats *SyStats) (ProtocolStats, error) {
	stats := ProtocolStats{
		Protocols: map[string]map[string]int64{},
		Time:      time.Now().Unix(),
	}

	// Both files share a format and carry disjoint protocol keys, so
	// they merge into one map. A missing file is skipped rather than
	// fatal.
	for _, path := range []string{systats.NetSNMPPath, systats.NetNetstatPath} {
		content, err := fileops.ReadFileWithError(path)
		if err != nil {
			continue
		}
		parseProcNetSNMP(content, stats.Protocols)
	}

	return stats, nil
}

// parseProcNetSNMP parses the paired-line format used by both
// /proc/net/snmp and /proc/net/netstat: a header line naming the
// columns, immediately followed by a value line, both prefixed with the
// same "Proto:" token.
//
//	Tcp: RtoAlgorithm RtoMin ... RetransSegs InErrs OutRsts
//	Tcp: 1 200 ... 41 0 55
//
// Pairing is by matching prefix, not by line order alone: if a value
// line is missing (a torn read), a naive alternating parser would zip
// Tcp's header against Udp's numbers and produce plausible-looking
// nonsense. Column counts also drift between kernel versions, so only
// min(header, values) columns are ever zipped.
func parseProcNetSNMP(content string, into map[string]map[string]int64) {
	pendingProto := ""
	var pendingHeader []string

	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasSuffix(fields[0], ":") {
			continue
		}
		proto := strings.TrimSuffix(fields[0], ":")
		cols := fields[1:]

		if pendingProto == proto {
			counters, ok := into[proto]
			if !ok {
				counters = map[string]int64{}
				into[proto] = counters
			}
			n := len(pendingHeader)
			if len(cols) < n {
				n = len(cols)
			}
			for i := 0; i < n; i++ {
				// ParseInt, not strops.ToUint64: Tcp's MaxConn is -1 on
				// virtually every host (it means "dynamic"), so these are
				// genuinely signed. Unparseable columns are skipped rather
				// than failing the whole file.
				if v, err := strconv.ParseInt(cols[i], 10, 64); err == nil {
					counters[pendingHeader[i]] = v
				}
			}
			pendingProto, pendingHeader = "", nil
			continue
		}

		// Not a match for the pending header - treat this as a new header
		// line, discarding any unpaired previous one.
		pendingProto, pendingHeader = proto, cols
	}
}
