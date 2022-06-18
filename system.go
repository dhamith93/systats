package systats

import (
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
	HostName      string
	OS            string
	Kernel        string
	UpTime        string
	LastBootDate  time.Time
	LoggedInUsers []User
	Time          int64
	TimeZone      string
}

// User holds logged in user information
type User struct {
	Username     string
	RemoteHost   string
	LoggedInTime time.Time
}

func getSystem(systats *SyStats) (System, error) {
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

	processLoggedInUsers(&output, systats)
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
		uptimeSecsFloat, err := strconv.ParseFloat(strings.TrimSpace(split[0]), 32)
		if err != nil {
			return err
		}
		uptime := time.Duration(int64(uptimeSecsFloat) * int64(time.Second))
		system.UpTime = strings.TrimSpace(uptime.String())
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

func processLoggedInUsers(system *System, systats *SyStats) {
	// NAME LINE TIME COMMENT
	split := strings.Split(exec.Execute("who"), "\n")
	system.LoggedInUsers = []User{}
	for _, line := range split {
		loggedInInfo := strings.Fields(line)
		if len(loggedInInfo) >= 5 {
			loggedInTime, _ := time.Parse("2006-01-02 15:04", loggedInInfo[2]+" "+loggedInInfo[3])
			system.LoggedInUsers = append(system.LoggedInUsers, User{
				Username:     loggedInInfo[0],
				LoggedInTime: loggedInTime,
				RemoteHost:   loggedInInfo[4],
			})
		}
	}
}
