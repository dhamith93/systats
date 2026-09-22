package systats

import (
	"context"
	"testing"
)

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

// A failed who(1) used to be parsed as login data: the old code used
// exec.Execute, which returns err.Error() in place of stdout, and
// `exec: "who": executable file not found in $PATH` has 8 whitespace-
// separated fields - enough to pass the field check and be read as a user
// "exec:" logged in from "not".
//
// Emptying PATH reproduces the distroless case exactly: exec.LookPath
// fails and the error text is what the old code parsed.
func TestProcessLoggedInUsersWhenWhoIsMissing(t *testing.T) {
	t.Setenv("PATH", "")

	var got System
	processLoggedInUsers(context.Background(), &got, &SyStats{})

	if len(got.LoggedInUsers) != 0 {
		t.Errorf("LoggedInUsers = %+v, want none when who(1) is not installed", got.LoggedInUsers)
	}
	if got.LoggedInUsers == nil {
		t.Errorf("LoggedInUsers = nil, want an empty slice for a clean JSON []")
	}
}

// A cancelled context is the other way the subprocess fails.
func TestProcessLoggedInUsersIgnoresExecFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var got System
	processLoggedInUsers(ctx, &got, &SyStats{})

	if len(got.LoggedInUsers) != 0 {
		t.Errorf("LoggedInUsers = %+v, want none when who(1) fails", got.LoggedInUsers)
	}
}

func TestParseWhoOutput(t *testing.T) {
	output := "dhamith  pts/0        2026-09-22 11:42 (192.168.1.50)\n" +
		"root     tty1         2026-09-21 08:03 (:0)\n"

	got := parseWhoOutput(output)
	if len(got) != 2 {
		t.Fatalf("got %d users, want 2: %+v", len(got), got)
	}
	if got[0].Username != "dhamith" || got[0].RemoteHost != "(192.168.1.50)" {
		t.Errorf("first user = %+v, want dhamith from (192.168.1.50)", got[0])
	}
	if got[0].LoggedInTime.IsZero() {
		t.Errorf("LoggedInTime is zero, want the parsed timestamp")
	}
	if got[1].Username != "root" {
		t.Errorf("second user = %q, want root", got[1].Username)
	}
}

// A local login with no comment column has only 4 fields and is skipped
// rather than indexed out of range.
func TestParseWhoOutputSkipsShortLines(t *testing.T) {
	if got := parseWhoOutput("dhamith  tty1  2026-09-22 11:42\n\n"); len(got) != 0 {
		t.Errorf("parseWhoOutput(short line) = %+v, want no users", got)
	}
}
