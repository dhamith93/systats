package systats

import "testing"

func TestProcessSystemBootTimes(t *testing.T) {
	syStats := &SyStats{
		UptimePath: "./test_files/uptime.txt",
		EtcPath:    "./test_files",
	}

	var got System
	if err := processSystemBootTimes(&got, syStats); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The fixture's first field is 12058.79. Parsed at bitSize 32 this
	// came back as 12058.7900390625.
	if want := 12058.79; got.UpTimeSeconds != want {
		t.Errorf("UpTimeSeconds = %v, want %v", got.UpTimeSeconds, want)
	}
	if want := "3h20m58s"; got.UpTime != want {
		t.Errorf("UpTime = %q, want %q", got.UpTime, want)
	}
	if got.LastBootDate.IsZero() {
		t.Errorf("LastBootDate is zero, want a real time")
	}
}
