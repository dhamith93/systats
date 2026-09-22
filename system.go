package systats

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/exec"
	"github.com/dhamith93/systats/internal/fileops"
)

// System holds operating system information
type System struct {
	HostName string `json:"hostName"`
	OS       string `json:"os"`
	Kernel   string `json:"kernel"`
	// UpTime is human-readable ("72h3m0s"). Use UpTimeSeconds for
	// arithmetic - parsing this back is lossy and needless.
	UpTime string `json:"upTime"`
	// UpTimeSeconds is the same value as a number, straight from
	// /proc/uptime's first field.
	UpTimeSeconds float64   `json:"upTimeSeconds"`
	LastBootDate  time.Time `json:"lastBootDate"`
	LoggedInUsers []User    `json:"loggedInUsers"`
	Time          int64     `json:"time"`
	TimeZone      string    `json:"timeZone"`
}

// User holds logged in user information
type User struct {
	Username     string    `json:"username"`
	RemoteHost   string    `json:"remoteHost"`
	LoggedInTime time.Time `json:"loggedInTime"`
}

func getSystem(ctx context.Context, systats *SyStats) (System, error) {
	output := System{}

	err := getOperatingSystem(&output, systats)
	if err != nil {
		return output, err
	}

	output.HostName = strings.TrimSpace(fileops.ReadFile(systats.EtcPath + "/hostname"))

	split := strings.Fields(fileops.ReadFile(systats.VersionPath))
	if len(split) >= 3 {
		output.Kernel = strings.TrimSpace(split[2])
	}

	err = processSystemBootTimes(&output, systats)
	if err != nil {
		return output, err
	}

	processLoggedInUsers(ctx, &output, systats)
	output.Time = time.Now().Unix()

	return output, nil
}

func getOperatingSystem(system *System, systats *SyStats) error {
	path := systats.EtcPath + "/os-release"
	content, err := fileops.ReadFileWithError(path)
	if err != nil {
		path, err = fileops.FindFileWithNameLike(systats.EtcPath, "-release")
		if err != nil {
			return err
		}
		content = fileops.ReadFile(path)
	}

	split := strings.Split(content, "\n")

	for _, line := range split {
		r, _ := regexp.Compile("^(PRETTY_NAME=\")(.+)(\")")
		matches := r.FindAllStringSubmatch(line, -1)
		if len(matches) > 0 && len(matches[0]) >= 3 {
			system.OS = matches[0][2]
		}
	}

	return nil
}

func processSystemBootTimes(system *System, systats *SyStats) error {
	split := strings.Fields(fileops.ReadFile(systats.UptimePath))
	if len(split) >= 1 {
		// bitSize 64, not 32: the value lands in a float64 field, and
		// float32 only carries ~7 significant digits - enough to distort
		// the uptime of a host that's been up for a few months.
		uptimeSecsFloat, err := strconv.ParseFloat(strings.TrimSpace(split[0]), 64)
		if err != nil {
			return err
		}
		uptime := time.Duration(int64(uptimeSecsFloat) * int64(time.Second))
		system.UpTime = strings.TrimSpace(uptime.String())
		system.UpTimeSeconds = uptimeSecsFloat
		system.LastBootDate = time.Now().Add(-uptime).Round(time.Second)
	}
	localTimePath, _ := os.Readlink(systats.EtcPath + "/localtime")
	split = strings.Split(localTimePath, "/")
	if len(split) >= 3 {
		if split[len(split)-2] == "zoneinfo" {
			system.TimeZone = split[len(split)-1]
		} else {
			system.TimeZone = split[len(split)-2] + "/" + split[len(split)-1]
		}
	}
	return nil
}

func processLoggedInUsers(ctx context.Context, system *System, systats *SyStats) {
	system.LoggedInUsers = []User{}

	// ExecuteWithErrorAndContext, not ExecuteWithContext: the latter
	// returns err.Error() in place of stdout, and `exec: "who":
	// executable file not found in $PATH` splits into 8 fields - enough
	// to satisfy the field check below and be parsed into a user named
	// "exec:" logged in from "not". A missing who(1) must yield no users,
	// not an invented one.
	output, err := exec.ExecuteWithErrorAndContext(ctx, "who")
	if err != nil {
		return
	}

	system.LoggedInUsers = parseWhoOutput(output)
}

// parseWhoOutput parses who(1)'s columns: NAME LINE TIME COMMENT, where
// TIME is two fields (date and time) and COMMENT holds the remote host.
// Lines with too few fields are skipped - who prints a header on some
// systems, and the comment column is absent for local logins.
func parseWhoOutput(output string) []User {
	users := []User{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		loggedInTime, _ := time.Parse("2006-01-02 15:04", fields[2]+" "+fields[3])
		users = append(users, User{
			Username:     fields[0],
			LoggedInTime: loggedInTime,
			RemoteHost:   fields[4],
		})
	}
	return users
}
