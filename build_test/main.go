package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/dhamith93/systats"
)

var (
	cpuProfile = flag.String("cpuprofile", "", "write a CPU profile to this file (analyze with: go tool pprof -http=:8080 <file>)")
	memProfile = flag.String("memprofile", "", "write a heap profile to this file after all calls complete")
)

// section is one Get* call's result, rendered as a labeled block in the
// HTML report - either its pretty-printed JSON, or the error it returned.
type section struct {
	Name     string
	OK       bool
	Error    string
	JSON     string
	Duration string
}

// check is a scalar (bool/int) call's result, rendered as a table row.
type check struct {
	Name     string
	Value    string
	Duration string
}

type reportData struct {
	GeneratedAt string
	Hostname    string
	GOOS        string
	Sections    []section
	Checks      []check
}

func main() {
	flag.Parse()

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			fmt.Println("could not create CPU profile:", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Println("could not start CPU profile:", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}

	syStats := systats.New()
	data := reportData{
		GeneratedAt: time.Now().Format(time.RFC1123),
		GOOS:        runtime.GOOS,
	}
	if h, err := os.Hostname(); err == nil {
		data.Hostname = h
	}

	data.Sections = append(data.Sections, timedCollect("GetSystem", func() (any, error) { return syStats.GetSystem() }))

	cpuStart := time.Now()
	cpuResult, cpuErr := syStats.GetCPU()
	cpuDuration := time.Since(cpuStart)
	data.Sections = append(data.Sections, buildSection("GetCPU", cpuResult, cpuErr, cpuDuration))
	if cpuErr == nil {
		data.Checks = append(data.Checks, check{
			Name:     "CPU load average (1m / 5m / 15m)",
			Value:    fmt.Sprintf("%.2f / %.2f / %.2f", cpuResult.Load1, cpuResult.Load5, cpuResult.Load15),
			Duration: cpuDuration.String(),
		})
	}

	data.Sections = append(data.Sections, timedCollect("GetMemory", func() (any, error) { return syStats.GetMemory(systats.Megabyte) }))
	data.Sections = append(data.Sections, timedCollect("GetSwap", func() (any, error) { return syStats.GetSwap(systats.Megabyte) }))
	data.Sections = append(data.Sections, timedCollect("GetDisks", func() (any, error) { return syStats.GetDisks() }))
	data.Sections = append(data.Sections, timedCollect("GetNetworks", func() (any, error) { return syStats.GetNetworks() }))

	// Same calls again with cgroup awareness on, so the report shows
	// host-wide vs container-relative numbers side by side. Limited/
	// AllocatedCores are the only exported signals of whether cgroup
	// detection actually found anything, so they get Checks rows too.
	syStats.ContainerAware = true

	cgMemStart := time.Now()
	cgMem, cgMemErr := syStats.GetMemory(systats.Megabyte)
	cgMemDuration := time.Since(cgMemStart)
	data.Sections = append(data.Sections, buildSection("GetMemory (container-aware)", cgMem, cgMemErr, cgMemDuration))
	if cgMemErr == nil {
		data.Checks = append(data.Checks, check{
			Name:     "Memory cgroup limit detected",
			Value:    fmt.Sprintf("%v (total: %d %s)", cgMem.Limited, cgMem.Total, cgMem.Unit),
			Duration: cgMemDuration.String(),
		})
	}

	cgCPUStart := time.Now()
	cgCPU, cgCPUErr := syStats.GetCPU()
	cgCPUDuration := time.Since(cgCPUStart)
	data.Sections = append(data.Sections, buildSection("GetCPU (container-aware)", cgCPU, cgCPUErr, cgCPUDuration))
	if cgCPUErr == nil {
		data.Checks = append(data.Checks, check{
			Name:     "CPU cgroup quota detected",
			Value:    fmt.Sprintf("%v (allocated cores: %.2f)", cgCPU.Limited, cgCPU.AllocatedCores),
			Duration: cgCPUDuration.String(),
		})
	}

	// The host-vs-container CPU% comparison is the clearest single
	// signal that quota-relative accounting works, so surface it as a
	// Checks row rather than leaving it buried in each section's JSON.
	if cpuErr == nil && cgCPUErr == nil {
		data.Checks = append(data.Checks, check{
			Name:     "CPU usage % (host-wide vs container-aware)",
			Value:    fmt.Sprintf("%d%% vs %d%%", cpuResult.LoadAvg, cgCPU.LoadAvg),
			Duration: (cpuDuration + cgCPUDuration).String(),
		})
	}

	syStats.ContainerAware = false

	syStats.ProcessCPUMode = systats.CPUUsageInstant
	data.Sections = append(data.Sections, timedCollect("GetTopProcesses(cpu, instant)", func() (any, error) { return syStats.GetTopProcesses(5, "cpu") }))
	data.Sections = append(data.Sections, timedCollect("GetTopProcesses(memory, instant)", func() (any, error) { return syStats.GetTopProcesses(5, "memory") }))

	syStats.ProcessCPUMode = systats.CPUUsageAverage
	data.Sections = append(data.Sections, timedCollect("GetTopProcesses(cpu, average)", func() (any, error) { return syStats.GetTopProcesses(5, "cpu") }))
	data.Sections = append(data.Sections, timedCollect("GetTopProcesses(memory, average)", func() (any, error) { return syStats.GetTopProcesses(5, "memory") }))

	data.Checks = append(data.Checks, timedCheck("IsServiceRunning(\"cron\")", func() string {
		return fmt.Sprintf("%v", syStats.IsServiceRunning("cron"))
	}))
	data.Checks = append(data.Checks, timedCheck("IsPortOpen(22)", func() string {
		return fmt.Sprintf("%v", syStats.IsPortOpen(22))
	}))
	data.Checks = append(data.Checks, timedCheck("CanConnectExternal(\"https://www.google.com\")", func() string {
		connected, connErr := syStats.CanConnectExternal("https://www.google.com")
		if connErr != nil {
			return fmt.Sprintf("%v (error: %s)", connected, connErr.Error())
		}
		return fmt.Sprintf("%v", connected)
	}))
	data.Checks = append(data.Checks, timedCheck("EstablishedTCPConnCount(\"sshd\")", func() string {
		return fmt.Sprintf("%v", syStats.EstablishedTCPConnCount("sshd"))
	}))

	// concise console summary - full detail goes into the HTML report
	for _, s := range data.Sections {
		status := "OK"
		if !s.OK {
			status = "ERROR: " + s.Error
		}
		fmt.Printf("%-34s %-10s %s\n", s.Name, s.Duration, status)
	}
	for _, c := range data.Checks {
		fmt.Printf("%-45s %-9s %s\n", c.Name, c.Duration, c.Value)
	}

	outPath, err := writeReport(data)
	if err != nil {
		fmt.Println("failed to write report:", err)
		os.Exit(1)
	}
	fmt.Println("\nReport written to:", outPath)

	if *memProfile != "" {
		f, err := os.Create(*memProfile)
		if err != nil {
			fmt.Println("could not create memory profile:", err)
			os.Exit(1)
		}
		defer f.Close()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Println("could not write memory profile:", err)
			os.Exit(1)
		}
		fmt.Println("Memory profile written to:", *memProfile)
	}
}

// timedCollect calls fn, measuring wall-clock duration around the call
// itself (excluding JSON marshaling), and builds the resulting section.
func timedCollect(name string, fn func() (any, error)) section {
	start := time.Now()
	value, err := fn()
	duration := time.Since(start)
	return buildSection(name, value, err, duration)
}

// buildSection is split out from timedCollect so callers that need the
// typed result (e.g. GetCPU, to also surface Load1/5/15 as a Checks row)
// can time/call it directly instead of through the any-erasing closure.
func buildSection(name string, value any, err error, duration time.Duration) section {
	s := section{Name: name, Duration: duration.String()}
	if err != nil {
		s.Error = err.Error()
		return s
	}
	b, jsonErr := json.MarshalIndent(value, "", "  ")
	if jsonErr != nil {
		s.Error = jsonErr.Error()
		return s
	}
	s.OK = true
	s.JSON = string(b)
	return s
}

// timedCheck calls fn, measuring wall-clock duration, and builds the
// resulting check row.
func timedCheck(name string, fn func() string) check {
	start := time.Now()
	value := fn()
	duration := time.Since(start)
	return check{Name: name, Value: value, Duration: duration.String()}
}

func writeReport(data reportData) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	outPath := filepath.Join(dir, "build_test", "report.html")

	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err := reportTemplate.Execute(f, data); err != nil {
		return "", err
	}
	return outPath, nil
}

var reportTemplate = template.Must(template.New("report").Parse(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>systats report</title>
<style>
	body { font-family: -apple-system, Helvetica, Arial, sans-serif; background: #f5f5f7; color: #1d1d1f; margin: 0; padding: 2rem; }
	h1 { font-size: 1.4rem; margin-bottom: 0.25rem; }
	.meta { color: #666; font-size: 0.85rem; margin-bottom: 1.5rem; }
	.card { background: #fff; border: 1px solid #e0e0e0; border-radius: 8px; margin-bottom: 1rem; overflow: hidden; }
	.card-header { display: flex; justify-content: space-between; align-items: center; padding: 0.6rem 1rem; background: #fafafa; border-bottom: 1px solid #e0e0e0; }
	.card-header h2 { font-size: 0.95rem; margin: 0; font-weight: 600; }
	.badge { font-size: 0.75rem; font-weight: 600; padding: 0.15rem 0.5rem; border-radius: 999px; }
	.badge-ok { background: #e3f7e8; color: #17803d; }
	.badge-error { background: #fde8e8; color: #c0261c; }
	pre { margin: 0; padding: 1rem; overflow-x: auto; font-size: 0.8rem; line-height: 1.4; }
	.error-text { padding: 1rem; color: #c0261c; font-size: 0.85rem; }
	table { width: 100%; border-collapse: collapse; }
	table td { padding: 0.5rem 1rem; border-bottom: 1px solid #eee; font-size: 0.85rem; }
	table tr:last-child td { border-bottom: none; }
	table td:first-child { color: #444; }
	table td:last-child { font-weight: 600; text-align: right; }
	.duration { color: #888; font-weight: 400; font-size: 0.8rem; margin-left: 0.6rem; }
</style>
</head>
<body>
	<h1>systats report</h1>
	<div class="meta">Generated {{.GeneratedAt}} on {{.Hostname}} ({{.GOOS}})</div>

	<div class="card">
		<div class="card-header"><h2>Checks</h2></div>
		<table>
		{{range .Checks}}
			<tr><td>{{.Name}} <span class="duration">{{.Duration}}</span></td><td>{{.Value}}</td></tr>
		{{end}}
		</table>
	</div>

	{{range .Sections}}
	<div class="card">
		<div class="card-header">
			<h2>{{.Name}} <span class="duration">{{.Duration}}</span></h2>
			{{if .OK}}<span class="badge badge-ok">OK</span>{{else}}<span class="badge badge-error">ERROR</span>{{end}}
		</div>
		{{if .OK}}
			<pre>{{.JSON}}</pre>
		{{else}}
			<div class="error-text">{{.Error}}</div>
		{{end}}
	</div>
	{{end}}
</body>
</html>
`))
