package systats

import (
	"errors"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/systats/internal/fileops"
	"github.com/dhamith93/systats/internal/strops"
)

// CPU holds information on CPU and CPU usage
type CPU struct {
	// LoadAvg and CoreAvg are host-wide CPU utilization percentages,
	// unless Limited, in which case LoadAvg is % of the cgroup's CPU
	// budget instead and CoreAvg is left nil (see Limited).
	LoadAvg int   `json:"loadAvg"`
	CoreAvg []int `json:"coreAvg"`
	// Load1/Load5/Load15 are the traditional Unix load average
	// (uptime/top/w), always host-wide regardless of ContainerAware -
	// there is no cgroup-scoped equivalent of this metric
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
	Model  string  `json:"model"`
	// NoOfCores is the host's logical CPU count - what nproc reports, and
	// always equal to len(CoreAvg) when CoreAvg is populated. Always
	// host-wide, in every mode; see AllocatedCores for the cgroup's
	// (possibly fractional) quota.
	NoOfCores int `json:"noOfCores"`
	// PhysicalCores is the host's distinct physical core count across all
	// sockets, counted from unique (physical id, core id) pairs. Many
	// hypervisors report the same pair for every vCPU, which correctly
	// yields 1 here - matching what lscpu shows on the same guest. Falls
	// back to the logical count on architectures that don't publish
	// topology (ARM, RISC-V, some containers).
	PhysicalCores int `json:"physicalCores"`
	// Sockets is the number of distinct physical packages, 1 when the
	// architecture doesn't publish topology.
	Sockets int    `json:"sockets"`
	Freq    string `json:"freq"`
	Cache   string `json:"cache"`
	Time    int64  `json:"time"`
	// Limited is true when SyStats.ContainerAware found a real cgroup
	// CPU quota, in which case LoadAvg reflects usage relative to that
	// quota (AllocatedCores) instead of the host's core count.
	Limited bool `json:"limited"`
	// AllocatedCores is the cgroup's CPU quota in cores (e.g. 0.5, 2.0),
	// which can be fractional (Kubernetes "500m" == 0.5). 0 when !Limited.
	AllocatedCores float64 `json:"allocatedCores"`
}

func getCPU(systats *SyStats, milliseconds int) (CPU, error) {
	output := CPU{}
	start := time.Now()

	statStr1, err := fileops.ReadFileWithError(systats.StatFilePath)
	if err != nil {
		return output, err
	}

	// Sample the cgroup's cumulative CPU usage counter alongside
	// /proc/stat, reusing this same sampling window rather than adding
	// a second sleep - the cgroup counter (like /proc/stat) has no
	// "total possible" companion value, so a delta over real elapsed
	// wall time is needed either way.
	var cgInfo cgroupCPUInfo
	var cgUsage1 float64
	var cgOK1 bool
	if systats.ContainerAware {
		cgInfo = resolveCgroupCPU(systats)
		if cgInfo.ok {
			cgUsage1, cgOK1 = sampleCgroupCPUUsageSeconds(cgInfo)
		}
	}

	// to calculate the cpu usage the /proc/stat has to be read some time apart
	time.Sleep(time.Duration(milliseconds) * time.Millisecond)
	statStr2, err := fileops.ReadFileWithError(systats.StatFilePath)
	if err != nil {
		return output, err
	}

	var cgUsage2 float64
	var cgOK2 bool
	if systats.ContainerAware && cgInfo.ok {
		cgUsage2, cgOK2 = sampleCgroupCPUUsageSeconds(cgInfo)
	}
	elapsedSeconds := time.Since(start).Seconds()

	processStatFileContents(&output, &statStr1, &statStr2)
	// Captured before applyCgroupCPU, which nils CoreAvg in limited mode -
	// this is the fallback logical-CPU count for architectures whose
	// /proc/cpuinfo has no "processor" lines at all (e.g. s390x).
	logicalFromStat := len(output.CoreAvg)
	if systats.ContainerAware {
		applyCgroupCPU(&output, cgInfo, cgUsage1, cgUsage2, cgOK1, cgOK2, elapsedSeconds)
	}

	cpuinfoStr, err := fileops.ReadFileWithError(systats.CPUinfoFilePath)
	if err != nil {
		return output, err
	}

	processCPUInfoFileContent(&output, &cpuinfoStr, logicalFromStat)

	loadAvgStr, err := fileops.ReadFileWithError(systats.LoadAvgPath)
	if err != nil {
		return output, err
	}
	if err := processLoadAvgFileContent(&output, &loadAvgStr); err != nil {
		return output, err
	}

	return output, nil
}

// cgroupCPUInfo is resolved once per getCPU call (not once per sample) -
// quotaDir holds cpu.max (v2) or cpu.cfs_quota_us/cpu.cfs_period_us (v1);
// usageDir holds cpu.stat (v2) or cpuacct.usage (v1). For v2 these are
// the same directory (single unified hierarchy); for v1 they're
// resolved as independent controllers since some distros mount
// cpu/cpuacct separately.
type cgroupCPUInfo struct {
	ok       bool
	limited  bool
	cores    float64
	version  cgroupVersion
	usageDir string
}

func resolveCgroupCPU(systats *SyStats) cgroupCPUInfo {
	version := detectCgroupVersion(systats.CgroupRootPath)
	if version == cgroupNone {
		return cgroupCPUInfo{}
	}

	var quotaDir, usageDir string
	if version == cgroupV2 {
		dir, ok := resolveCgroupControllerPath(systats, version, "")
		if !ok {
			return cgroupCPUInfo{}
		}
		quotaDir, usageDir = dir, dir
	} else {
		cpuDir, cpuOK := resolveCgroupControllerPath(systats, version, "cpu")
		cpuacctDir, cpuacctOK := resolveCgroupControllerPath(systats, version, "cpuacct")
		if !cpuOK || !cpuacctOK {
			return cgroupCPUInfo{}
		}
		quotaDir, usageDir = cpuDir, cpuacctDir
	}

	cores, limited, err := readCgroupCPUQuota(version, quotaDir)
	if err != nil {
		return cgroupCPUInfo{}
	}

	return cgroupCPUInfo{ok: true, limited: limited, cores: cores, version: version, usageDir: usageDir}
}

// readCgroupCPUQuota reads dir's CPU quota. limited is false when no
// real quota is configured (v2's literal "max", or v1's quota <= 0,
// conventionally -1 for unlimited).
func readCgroupCPUQuota(version cgroupVersion, dir string) (cores float64, limited bool, err error) {
	switch version {
	case cgroupV2:
		content, err := fileops.ReadFileWithError(path.Join(dir, "cpu.max"))
		if err != nil {
			return 0, false, err
		}
		fields := strings.Fields(content)
		if len(fields) < 2 {
			return 0, false, errors.New("unexpected cpu.max format")
		}
		if fields[0] == "max" {
			return 0, false, nil
		}
		period := strops.ToFloat64(fields[1])
		if period <= 0 {
			return 0, false, nil
		}
		return strops.ToFloat64(fields[0]) / period, true, nil
	case cgroupV1:
		quotaContent, err := fileops.ReadFileWithError(path.Join(dir, "cpu.cfs_quota_us"))
		if err != nil {
			return 0, false, err
		}
		// quota is a signed value (-1 == unlimited, valid data, not
		// corruption), so it's parsed directly rather than via strops
		// (unsigned, panics on anything it can't parse as uint64).
		quota, err := strconv.ParseInt(strings.TrimSpace(quotaContent), 10, 64)
		if err != nil {
			return 0, false, err
		}
		if quota <= 0 {
			return 0, false, nil
		}
		periodContent, err := fileops.ReadFileWithError(path.Join(dir, "cpu.cfs_period_us"))
		if err != nil {
			return 0, false, err
		}
		period := strops.ToFloat64(strings.TrimSpace(periodContent))
		if period <= 0 {
			return 0, false, nil
		}
		return float64(quota) / period, true, nil
	default:
		return 0, false, errors.New("unknown cgroup version")
	}
}

// sampleCgroupCPUUsageSeconds reads info's cumulative CPU usage counter,
// normalized to seconds regardless of version (v1's cpuacct.usage is
// nanoseconds, v2's cpu.stat usage_usec is microseconds) so nothing
// downstream needs to know which version it came from.
func sampleCgroupCPUUsageSeconds(info cgroupCPUInfo) (float64, bool) {
	if !info.ok {
		return 0, false
	}
	switch info.version {
	case cgroupV2:
		content, err := fileops.ReadFileWithError(path.Join(info.usageDir, "cpu.stat"))
		if err != nil {
			return 0, false
		}
		usec, ok := parseCPUStatUsageUsec(content)
		if !ok {
			return 0, false
		}
		return float64(usec) / 1e6, true
	case cgroupV1:
		content, err := fileops.ReadFileWithError(path.Join(info.usageDir, "cpuacct.usage"))
		if err != nil {
			return 0, false
		}
		return float64(strops.ToUint64(strings.TrimSpace(content))) / 1e9, true
	default:
		return 0, false
	}
}

func parseCPUStatUsageUsec(content string) (uint64, bool) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, false
			}
			return value, true
		}
	}
	return 0, false
}

// applyCgroupCPU overrides output's LoadAvg/CoreAvg with cgroup-relative
// values when a real quota was found and both samples succeeded. Any
// failure leaves output's host-wide values (already computed by
// processStatFileContents) untouched - this never changes GetCPU's
// error behavior or return shape.
func applyCgroupCPU(output *CPU, info cgroupCPUInfo, usageSeconds1, usageSeconds2 float64, ok1, ok2 bool, elapsedSeconds float64) {
	if !info.ok {
		return
	}
	output.Limited = info.limited
	output.AllocatedCores = info.cores
	if !info.limited || !ok1 || !ok2 || elapsedSeconds <= 0 {
		return
	}
	coresUsed := (usageSeconds2 - usageSeconds1) / elapsedSeconds
	output.LoadAvg = int(100 * coresUsed / info.cores)
	output.CoreAvg = nil
}

// processLoadAvgFileContent parses /proc/loadavg's 1/5/15-minute load
// averages - the traditional Unix "load average" (uptime/top/w), distinct
// from LoadAvg/CoreAvg above which are CPU utilization percentages.
func processLoadAvgFileContent(output *CPU, content *string) error {
	fields := strings.Fields(*content)
	if len(fields) < 3 {
		return errors.New("unexpected loadavg format")
	}
	output.Load1 = strops.ToFloat64(fields[0])
	output.Load5 = strops.ToFloat64(fields[1])
	output.Load15 = strops.ToFloat64(fields[2])
	return nil
}

// processCPUInfoFileContent fills in the descriptive CPU fields and the
// core counts. logicalFallback is used only when /proc/cpuinfo has no
// "processor" lines at all (s390x uses a different layout, and some
// minimal containers mask the file) - callers pass the count derived
// from /proc/stat's per-CPU lines.
func processCPUInfoFileContent(output *CPU, content *string, logicalFallback int) {
	split := strings.Split(*content, "\n")

	for _, line := range split {
		lineArr := strings.Fields(line)

		if len(lineArr) == 0 {
			continue
		}

		if len(lineArr) > 3 && (lineArr[0]+lineArr[1] == "modelname") {
			name := ""
			for i := 3; i < len(lineArr); i++ {
				name += lineArr[i] + " "
			}
			output.Model = strings.TrimSpace(name)
			continue
		}

		if len(lineArr) > 3 && (lineArr[0]+lineArr[1] == "cpuMHz") {
			output.Freq = strings.TrimSpace(lineArr[3]) + " MHz"
			continue
		}

		if len(lineArr) > 3 && (lineArr[0]+lineArr[1] == "cachesize") {
			name := ""
			for i := 3; i < len(lineArr); i++ {
				name += lineArr[i] + " "
			}
			output.Cache = strings.TrimSpace(name)
			continue
		}
	}

	topo := parseCPUTopology(*content)
	if topo.logical == 0 {
		topo.logical = logicalFallback
		topo.physicalCores = logicalFallback
		topo.sockets = 1
	}
	output.NoOfCores = topo.logical
	output.PhysicalCores = topo.physicalCores
	output.Sockets = topo.sockets

	output.Time = time.Now().Unix()
}

// cpuTopology counts logical CPUs, distinct physical cores, and sockets
// from /proc/cpuinfo. /proc/cpuinfo's "cpu cores" field is deliberately
// not used: it reports physical cores *per socket*, so it's half the
// real count on a dual-socket box, and on hybrid P+E-core parts it
// describes neither core type correctly. Distinct (physical id, core id)
// pairs are the only count that's right in every case.
type cpuTopology struct {
	logical       int
	physicalCores int
	sockets       int
}

type coreKey struct{ pkg, core string }

func parseCPUTopology(content string) cpuTopology {
	seenCores := map[coreKey]bool{}
	seenPkgs := map[string]bool{}

	logical := 0
	var pkg, core string
	var havePkg, haveCore bool

	flush := func() {
		if havePkg {
			seenPkgs[pkg] = true
			if haveCore {
				seenCores[coreKey{pkg, core}] = true
			}
		}
		pkg, core, havePkg, haveCore = "", "", false, false
	}

	for _, line := range strings.Split(content, "\n") {
		// strings.Cut rather than Fields: values can be empty
		// ("power management:"), which a field-count guard would drop.
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "processor":
			flush() // close out the previous block
			logical++
		case "physical id":
			pkg, havePkg = v, true
		case "core id":
			core, haveCore = v, true
		}
	}
	flush() // close out the last block - there's no trailing "processor"

	t := cpuTopology{logical: logical, physicalCores: len(seenCores), sockets: len(seenPkgs)}
	// ARM, RISC-V, and some VMs/containers omit physical id and core id
	// entirely. There's no topology to report, so treat every logical CPU
	// as its own core on a single socket rather than reporting zero.
	if t.physicalCores == 0 {
		t.physicalCores = t.logical
	}
	if t.sockets == 0 && t.logical > 0 {
		t.sockets = 1
	}
	return t
}

func processStatFileContents(output *CPU, statStr1 *string, statStr2 *string) {
	statArr1 := processStatFile(statStr1)
	statArr2 := processStatFile(statStr2)

	for i := range statArr1 {
		// user + system, and user+system+idle times
		a1 := statArr1[i][0] + statArr1[i][1]
		a2 := statArr1[i][0] + statArr1[i][1] + statArr1[i][2]
		b := (statArr2[i][0] + statArr2[i][1] + statArr2[i][2] - a2)
		if b > 0 {
			usage := 100 * (statArr2[i][0] + statArr2[i][1] - a1) / b
			if i == 0 {
				output.LoadAvg = int(usage)
				continue
			}
			output.CoreAvg = append(output.CoreAvg, int(usage))
		} else {
			output.CoreAvg = append(output.CoreAvg, 0)
		}
	}
}

func processStatFile(content *string) [][]uint64 {
	statSplit := strings.Split(*content, "\n")
	output := [][]uint64{}
	r, _ := regexp.Compile("^cpu")

	for _, line := range statSplit {
		lineArr := strings.Fields(line)
		if len(lineArr) > 0 && r.MatchString(lineArr[0]) {
			output = append(output, []uint64{strops.ToUint64(lineArr[1]), strops.ToUint64(lineArr[3]), strops.ToUint64(lineArr[4])})
		}
	}

	return output
}
