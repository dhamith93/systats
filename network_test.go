package systats

import (
	"testing"

	"github.com/dhamith93/systats/internal/fileops"
)

func tcpFixtureStats() *SyStats {
	return &SyStats{
		NetTCPPath:  "./test_files/net_tcp.txt",
		NetTCP6Path: "./test_files/net_tcp6.txt",
	}
}

func TestParseProcNetTCP(t *testing.T) {
	records := parseProcNetTCP(fileops.ReadFile("./test_files/net_tcp.txt"))

	// 12 data rows, but the last is truncated to 3 fields and must be
	// skipped rather than parsed into garbage.
	if len(records) != 11 {
		t.Fatalf("got %d records, want 11 (the truncated row should be skipped)", len(records))
	}
	if records[0].state != "0A" || records[0].inode != "18443" {
		t.Errorf("first record = %+v, want state 0A inode 18443", records[0])
	}
	if records[2].state != "01" || records[2].inode != "22311" {
		t.Errorf("third record = %+v, want state 01 inode 22311", records[2])
	}
}

func TestParseProcNetTCPSkipsHeader(t *testing.T) {
	for _, r := range parseProcNetTCP(fileops.ReadFile("./test_files/net_tcp.txt")) {
		if r.state == "st" {
			t.Errorf("header row leaked through as a record")
		}
	}
}

func TestTCPStateName(t *testing.T) {
	cases := map[string]string{
		"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV",
		"04": "FIN_WAIT1", "05": "FIN_WAIT2", "06": "TIME_WAIT",
		"07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK",
		"0A": "LISTEN", "0B": "CLOSING", "0C": "NEW_SYN_RECV",
		"0F": "UNKNOWN", "": "UNKNOWN",
		// the kernel emits uppercase, but be tolerant
		"0a": "LISTEN",
	}
	for hex, want := range cases {
		if got := tcpStateName(hex); got != want {
			t.Errorf("tcpStateName(%q) = %q, want %q", hex, got, want)
		}
	}
}

func TestGetTCPConnectionStates(t *testing.T) {
	got, err := getTCPConnectionStates(tcpFixtureStats())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// IPv4 fixture: 2 listen, 3 established, 3 time-wait, 1 close-wait,
	// 1 fin-wait1, 1 unknown. IPv6 adds 1 listen, 1 established,
	// 1 time-wait - proving both files are summed, not overwritten.
	checks := map[string]struct{ got, want int }{
		"Established": {got.Established, 4},
		"Listen":      {got.Listen, 3},
		"TimeWait":    {got.TimeWait, 4},
		"CloseWait":   {got.CloseWait, 1},
		"FinWait1":    {got.FinWait1, 1},
		"Unknown":     {got.Unknown, 1},
		"Total":       {got.Total, 14},
	}
	for name, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", name, c.got, c.want)
		}
	}

	sum := got.Established + got.SynSent + got.SynRecv + got.FinWait1 +
		got.FinWait2 + got.TimeWait + got.Close + got.CloseWait +
		got.LastAck + got.Listen + got.Closing + got.NewSynRecv + got.Unknown
	if sum != got.Total {
		t.Errorf("buckets sum to %d but Total is %d - they must always reconcile", sum, got.Total)
	}
	if got.Time == 0 {
		t.Errorf("Time not set")
	}
}

func TestGetTCPConnectionStatesMissingIPv6(t *testing.T) {
	// An IPv6-disabled host has no /proc/net/tcp6 at all; that's not an
	// error, it just contributes nothing.
	syStats := &SyStats{
		NetTCPPath:  "./test_files/net_tcp.txt",
		NetTCP6Path: "./test_files/does_not_exist.txt",
	}
	got, err := getTCPConnectionStates(syStats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Total != 11 {
		t.Errorf("Total = %d, want 11 (IPv4 only)", got.Total)
	}
}

// TestEstablishedTCPInodesAfterRefactor guards the behavior of the
// pre-existing established-only path, which now shares parseProcNetTCP
// with the state summary.
func TestEstablishedTCPInodesAfterRefactor(t *testing.T) {
	got := establishedTCPInodes(tcpFixtureStats())

	want := []string{"22311", "24518", "24519", "31002"}
	if len(got) != len(want) {
		t.Fatalf("got %d inodes, want %d: %v", len(got), len(want), got)
	}
	for _, inode := range want {
		if !got[inode] {
			t.Errorf("inode %s missing from the established set", inode)
		}
	}
	// TIME_WAIT rows carry inode 0 and must not be counted as established.
	if got["0"] {
		t.Errorf("inode 0 (TIME_WAIT) should not be in the established set")
	}
}

func sysClassNetFixtureStats() *SyStats {
	return &SyStats{SysClassNetPath: "./test_files/sys_class_net"}
}

func TestGetNetworkUsageFromFixture(t *testing.T) {
	got := getNetworkUsage(sysClassNetFixtureStats(), "eth0")

	if got.State != "up" {
		t.Errorf("State = %q, want %q", got.State, "up")
	}
	if got.RxBytes != 1234567890 {
		t.Errorf("RxBytes = %d, want 1234567890", got.RxBytes)
	}
	if got.TxBytes != 987654321 {
		t.Errorf("TxBytes = %d, want 987654321", got.TxBytes)
	}
	if got.RxPackets != 4500123 {
		t.Errorf("RxPackets = %d, want 4500123", got.RxPackets)
	}
	if got.TxPackets != 3200456 {
		t.Errorf("TxPackets = %d, want 3200456", got.TxPackets)
	}
}

// Counters a kernel doesn't publish read as zero rather than failing the
// whole call - wlan0's fixture has operstate and rx_bytes only.
func TestGetNetworkUsagePartialCounters(t *testing.T) {
	got := getNetworkUsage(sysClassNetFixtureStats(), "wlan0")

	if got.State != "down" {
		t.Errorf("State = %q, want %q", got.State, "down")
	}
	if got.TxBytes != 0 || got.RxPackets != 0 {
		t.Errorf("missing counters should read 0, got %+v", got)
	}
}

// An interface with no sysfs directory at all must not panic.
func TestGetNetworkUsageUnknownInterface(t *testing.T) {
	got := getNetworkUsage(sysClassNetFixtureStats(), "nope0")

	if got != (NetworkUsage{}) {
		t.Errorf("unknown interface = %+v, want the zero value", got)
	}
}
