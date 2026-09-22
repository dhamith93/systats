package systats

import (
	"testing"

	"github.com/dhamith93/systats/internal/fileops"
)

func TestParseProcNetSNMP(t *testing.T) {
	stats := map[string]map[string]int64{}
	parseProcNetSNMP(fileops.ReadFile("./test_files/net_snmp.txt"), stats)

	cases := []struct {
		proto, counter string
		want           int64
	}{
		// MaxConn is -1 on virtually every Linux host. This is the case
		// that makes these values signed - strops.ToUint64 would panic.
		{"Tcp", "MaxConn", -1},
		{"Tcp", "RetransSegs", 4127},
		{"Tcp", "InErrs", 6},
		{"Tcp", "CurrEstab", 42},
		{"Udp", "NoPorts", 812},
		{"Udp", "InErrors", 4},
		{"Udp", "IgnoredMulti", 22},
		{"Ip", "InReceives", 1543201},
		{"Icmp", "InDestUnreachs", 120},
		// IcmpMsg's columns are whatever ICMP types the host has seen -
		// exactly why this is a map and not a struct.
		{"IcmpMsg", "OutType8", 343},
		{"IcmpMsg", "InType3", 120},
	}

	for _, c := range cases {
		got, ok := stats[c.proto][c.counter]
		if !ok {
			t.Errorf("%s.%s missing", c.proto, c.counter)
			continue
		}
		if got != c.want {
			t.Errorf("%s.%s = %d, want %d", c.proto, c.counter, got, c.want)
		}
	}
}

// TestParseProcNetSNMPMismatchedColumns covers the two torn-read traps: a
// value line shorter than its header, and a header with no value line at
// all followed by a different protocol. A naive alternating parser zips
// the orphaned header against the next protocol's numbers and produces
// plausible-looking nonsense.
func TestParseProcNetSNMPMismatchedColumns(t *testing.T) {
	stats := map[string]map[string]int64{}
	parseProcNetSNMP(fileops.ReadFile("./test_files/net_snmp_short_value_line.txt"), stats)

	// Tcp's header has 9 columns but only 5 values - zip the 5, drop the rest.
	if len(stats["Tcp"]) != 5 {
		t.Errorf("Tcp has %d counters, want 5: %v", len(stats["Tcp"]), stats["Tcp"])
	}
	if stats["Tcp"]["MaxConn"] != -1 {
		t.Errorf("Tcp.MaxConn = %d, want -1", stats["Tcp"]["MaxConn"])
	}
	if _, ok := stats["Tcp"]["RetransSegs"]; ok {
		t.Errorf("Tcp.RetransSegs should be absent - it had no value column")
	}

	// Icmp's header has no value line. It must be dropped entirely, not
	// paired with Udp's numbers.
	if _, ok := stats["Icmp"]; ok {
		t.Errorf("Icmp should be absent entirely, got %v", stats["Icmp"])
	}

	// Udp must still parse correctly despite the orphaned header before it.
	if stats["Udp"]["InDatagrams"] != 120134 || stats["Udp"]["OutDatagrams"] != 118800 {
		t.Errorf("Udp mis-parsed: %v", stats["Udp"])
	}
}

func TestGetProtocolStatsMergesBothFiles(t *testing.T) {
	syStats := &SyStats{
		NetSNMPPath:    "./test_files/net_snmp.txt",
		NetNetstatPath: "./test_files/net_netstat.txt",
	}
	got, err := getProtocolStats(syStats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// From /proc/net/snmp
	if v, ok := got.Value("Tcp", "RetransSegs"); !ok || v != 4127 {
		t.Errorf("Tcp.RetransSegs = %d (ok=%v), want 4127", v, ok)
	}
	// From /proc/net/netstat - disjoint protocol keys, same parser
	if v, ok := got.Value("TcpExt", "ListenOverflows"); !ok || v != 37 {
		t.Errorf("TcpExt.ListenOverflows = %d (ok=%v), want 37", v, ok)
	}
	if v, ok := got.Value("TcpExt", "TCPSynRetrans"); !ok || v != 512 {
		t.Errorf("TcpExt.TCPSynRetrans = %d (ok=%v), want 512", v, ok)
	}
	if v, ok := got.Value("IpExt", "InOctets"); !ok || v != 8822143021 {
		t.Errorf("IpExt.InOctets = %d (ok=%v), want 8822143021", v, ok)
	}
	if got.Time == 0 {
		t.Errorf("Time not set")
	}
}

func TestProtocolStatsValueMissing(t *testing.T) {
	stats := ProtocolStats{Protocols: map[string]map[string]int64{
		"Tcp": {"RetransSegs": 10},
	}}

	if _, ok := stats.Value("Nope", "RetransSegs"); ok {
		t.Errorf("missing protocol should report ok=false")
	}
	// The whole point of the (value, ok) shape: "this kernel doesn't
	// implement that counter" must be distinguishable from "it's zero".
	if _, ok := stats.Value("Tcp", "NotACounter"); ok {
		t.Errorf("missing counter should report ok=false")
	}
	if v, ok := stats.Value("Tcp", "RetransSegs"); !ok || v != 10 {
		t.Errorf("present counter = %d (ok=%v), want 10 true", v, ok)
	}
}

func TestGetProtocolStatsMissingFilesIsNotFatal(t *testing.T) {
	syStats := &SyStats{
		NetSNMPPath:    "./test_files/does_not_exist.txt",
		NetNetstatPath: "./test_files/does_not_exist.txt",
	}
	got, err := getProtocolStats(syStats)
	if err != nil {
		t.Fatalf("missing files should not be fatal, got %v", err)
	}
	if len(got.Protocols) != 0 {
		t.Errorf("want no protocols, got %v", got.Protocols)
	}
}
