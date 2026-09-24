package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dhamith93/systats"
)

// Dashboard is everything one page render needs. Every field is already
// formatted or pre-computed - html/template can't do arithmetic, so the
// geometry and the unit conversions happen here and in render.go rather
// than in the markup.
type Dashboard struct {
	GeneratedAt string
	Elapsed     string

	Host      HostPanel
	CPU       CPUPanel
	Memory    GaugePanel
	Swap      GaugePanel
	Container *ContainerPanel // nil unless a cgroup limit was found
	// Containers are the other containers on this host, one card each.
	// Distinct from Container above, which is this process's own cgroup.
	Containers     []ContainerCard
	ContainersNote string
	Pressure       PressurePanel
	Disks          []DiskPanel
	DiskIO         []DiskIOPanel
	Networks       []NetworkPanel
	TCP            TCPPanel
	Temps          []TempPanel
	Processes      []ProcessRow

	// Problems collects per-panel failures instead of aborting the whole
	// page. On a non-Linux host almost everything lands here, and the
	// dashboard still renders - which is the point.
	Problems []Problem
}

type Problem struct {
	Panel string
	Err   string
}

type HostPanel struct {
	OK            bool
	Hostname      string
	OS            string
	Kernel        string
	Uptime        string
	TimeZone      string
	LoggedInUsers int
}

// CPUPanel carries the load averages and topology as well as utilisation,
// because they all come from GetCPU. Keeping them here rather than on
// HostPanel means each panel is written by exactly one goroutine.
type CPUPanel struct {
	OK         bool
	Gauge      Gauge
	Cores      []Bar
	Sockets    int
	Model      string
	CoresLabel string
	Load1      string
	Load5      string
	Load15     string
}

// GaugePanel is the shared shape for memory and swap: a donut plus the
// absolute figures beside it.
type GaugePanel struct {
	OK      bool
	Gauge   Gauge
	Used    string
	Total   string
	Free    string
	Detail  string
	Present bool // swap is legitimately absent on many hosts
}

// ContainerPanel is populated only when systats finds a cgroup limit that
// applies to this process. It is the clearest demonstration of why
// ContainerAware exists: Host* is what /proc says, Limit* is the truth.
type ContainerPanel struct {
	MemoryLimited  bool
	MemoryLimit    string
	MemoryHost     string
	MemoryGauge    Gauge
	CPULimited     bool
	AllocatedCores string
	CPUGauge       Gauge
	HostCores      int
}

// ContainerCard is one container on this host, from GetContainers. The
// rates come from two samples diffed with Container.RatesSince.
type ContainerCard struct {
	Name     string
	ShortID  string
	Image    string
	Runtime  string
	State    string
	Running  bool
	CPUGauge Gauge
	MemGauge Gauge
	CPU      string
	Memory   string
	RxRate   string
	TxRate   string
	Read     string
	Write    string
	Pids     Bar
	// Throttled and OOMKills are the "something is wrong" counters: a
	// container pinned at its CPU quota, or one losing processes to the
	// OOM killer.
	Throttled string
	OOMKills  uint64
	// Layer is the container's own disk usage (its writable layer), or
	// why it wasn't measured.
	Layer  string
	Mounts []ContainerMountRow
	Notes  []string
}

type ContainerMountRow struct {
	MountPoint string
	Bar        Bar
	Usage      string
}

type PressurePanel struct {
	Available bool
	Note      string
	Limited   bool
	Resources []PressureRow
}

type PressureRow struct {
	Name string
	// Some is "at least one task stalled", Full is "nothing ran at all".
	Some          []Bar
	Full          []Bar
	FullAvailable bool
	SomeTotal     string
}

type DiskPanel struct {
	Device     string
	MountedOn  string
	Type       string
	UsedPct    float64
	Bar        Bar
	Used       string
	Size       string
	Available  string
	InodePct   string
	InodeUsage string
}

type DiskIOPanel struct {
	Device    string
	ReadRate  string
	WriteRate string
	IOPS      string
	Util      Bar
	InFlight  uint64
}

type NetworkPanel struct {
	Interface string
	State     string
	Up        bool
	IP        string
	IPv6      string
	MAC       string
	RxTotal   string
	TxTotal   string
	RxRate    string
	TxRate    string
	Packets   string
}

type TCPPanel struct {
	OK       bool
	Total    int
	Segments []Segment
	Rows     []LabelValue
}

type TempPanel struct {
	Chip     string
	Label    string
	Celsius  string
	Gauge    Gauge
	Limits   string
	Critical bool
}

type ProcessRow struct {
	Pid      int
	Name     string
	User     string
	State    string
	Threads  int
	CPU      string
	CPUBar   Bar
	Mem      string
	MemBar   Bar
	OpenFDs  string
	ExecPath string
}

type LabelValue struct {
	Label string
	Value string
}

// collect gathers everything for one render.
//
// The independent readers run concurrently: a SyStats is safe to share
// across goroutines once configured (no method writes to its receiver),
// so this is both faster and a demonstration of that contract.
//
// The one trap worth noting: ContainerAware is an ordinary mutable field,
// so the container-aware reads get their own SyStats value rather than
// flipping the shared one. Toggling a shared instance while another
// goroutine is mid-call is a data race.
func collect(ctx context.Context, cfg Config) Dashboard {
	start := time.Now()

	host := systats.New()
	host.CPUSampleWindow = cfg.SampleWindow
	// Lifetime-average CPU per process: a single /proc read each, versus a
	// second sampling window per call in the default instant mode. On real
	// hardware that is ~6ms instead of ~320ms for the same table.
	host.ProcessCPUMode = systats.CPUUsageAverage
	host.ContainerSocketPath = cfg.ContainerSocket
	host.ContainerLayerSize = cfg.ContainerLayers

	// Separate value, not host with a flag flipped - see the note above.
	cgroup := systats.New()
	cgroup.CPUSampleWindow = cfg.SampleWindow
	cgroup.ContainerAware = true

	d := Dashboard{GeneratedAt: time.Now().Format("Mon 02 Jan 2006 15:04:05 MST")}

	var mu sync.Mutex
	fail := func(panel string, err error) {
		mu.Lock()
		defer mu.Unlock()
		d.Problems = append(d.Problems, Problem{Panel: panel, Err: err.Error()})
	}

	var wg sync.WaitGroup
	run := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}

	var (
		hostCPU systats.CPU
		hostMem systats.Memory
		cgMem   systats.Memory
		cgCPU   systats.CPU
	)

	run(func() {
		system, err := host.GetSystemWithContext(ctx)
		if err != nil {
			fail("System", err)
			return
		}
		d.Host = buildHostPanel(system)
	})

	run(func() {
		cpu, err := host.GetCPUWithContext(ctx)
		if err != nil {
			fail("CPU", err)
			return
		}
		mu.Lock()
		hostCPU = cpu
		mu.Unlock()
		d.CPU = buildCPUPanel(cpu)
	})

	run(func() {
		mem, err := host.GetMemory(cfg.Unit)
		if err != nil {
			fail("Memory", err)
			return
		}
		mu.Lock()
		hostMem = mem
		mu.Unlock()
		d.Memory = buildMemoryPanel(mem)
	})

	run(func() {
		swap, err := host.GetSwap(cfg.Unit)
		if err != nil {
			fail("Swap", err)
			return
		}
		d.Swap = buildSwapPanel(swap)
	})

	run(func() {
		p, err := cgroup.GetPressure()
		if err != nil {
			fail("Pressure", err)
			return
		}
		d.Pressure = buildPressurePanel(p)
	})

	run(func() {
		disks, err := host.GetDisks()
		if err != nil {
			fail("Disks", err)
			return
		}
		d.Disks = buildDiskPanels(disks, cfg.Unit)
	})

	run(func() {
		io, err := sampleDiskIO(ctx, &host, cfg.SampleInterval)
		if err != nil {
			fail("Disk I/O", err)
			return
		}
		d.DiskIO = io
	})

	run(func() {
		nets, err := sampleNetworks(ctx, &host, cfg.SampleInterval)
		if err != nil {
			fail("Networks", err)
			return
		}
		d.Networks = nets
	})

	run(func() {
		states, err := host.GetTCPConnectionStates()
		if err != nil {
			fail("TCP states", err)
			return
		}
		d.TCP = buildTCPPanel(states)
	})

	run(func() {
		temps, err := host.GetTemperatures()
		if err != nil {
			fail("Temperatures", err)
			return
		}
		d.Temps = buildTempPanels(temps)
	})

	run(func() {
		procs, err := host.GetTopProcessesWithContext(ctx, cfg.TopProcesses, systats.SortByCPU)
		if err != nil {
			fail("Processes", err)
			return
		}
		d.Processes = buildProcessRows(procs)
	})

	run(func() {
		cards, note, err := sampleContainers(ctx, &host, cfg.Unit, cfg.SampleInterval)
		if err != nil {
			fail("Containers", err)
			return
		}
		d.Containers, d.ContainersNote = cards, note
	})

	run(func() {
		mem, err := cgroup.GetMemory(cfg.Unit)
		if err != nil {
			fail("Memory (container-aware)", err)
			return
		}
		mu.Lock()
		cgMem = mem
		mu.Unlock()
	})

	run(func() {
		cpu, err := cgroup.GetCPUWithContext(ctx)
		if err != nil {
			fail("CPU (container-aware)", err)
			return
		}
		mu.Lock()
		cgCPU = cpu
		mu.Unlock()
	})

	wg.Wait()

	// Only worth a panel when a real limit was found. Both Limited flags
	// are false on a bare host, which is the normal case.
	if cgMem.Limited || cgCPU.Limited {
		d.Container = buildContainerPanel(cgMem, cgCPU, hostMem, hostCPU, cfg.Unit)
	}

	sort.Slice(d.Problems, func(i, j int) bool { return d.Problems[i].Panel < d.Problems[j].Panel })
	d.Elapsed = time.Since(start).Round(time.Millisecond).String()
	return d
}

func buildHostPanel(s systats.System) HostPanel {
	return HostPanel{
		OK:            true,
		Hostname:      s.HostName,
		OS:            s.OS,
		Kernel:        s.Kernel,
		Uptime:        s.UpTime,
		TimeZone:      s.TimeZone,
		LoggedInUsers: len(s.LoggedInUsers),
	}
}

func buildCPUPanel(c systats.CPU) CPUPanel {
	p := CPUPanel{
		OK:      true,
		Gauge:   newGauge(float64(c.LoadAvg), fmt.Sprintf("%d%%", c.LoadAvg), "CPU"),
		Sockets: c.Sockets,
		Model:   c.Model,
		// Load1/5/15 are the traditional Unix load average, not a
		// percentage - they can exceed the core count and are a different
		// metric from LoadAvg above.
		Load1:  fmt.Sprintf("%.2f", c.Load1),
		Load5:  fmt.Sprintf("%.2f", c.Load5),
		Load15: fmt.Sprintf("%.2f", c.Load15),
		// NoOfCores is logical CPUs (what nproc reports); PhysicalCores
		// is distinct cores across all sockets.
		CoresLabel: fmt.Sprintf("%d logical / %d physical", c.NoOfCores, c.PhysicalCores),
	}
	for i, core := range c.CoreAvg {
		p.Cores = append(p.Cores, newBar(float64(core), fmt.Sprintf("cpu%d", i), fmt.Sprintf("%d%%", core)))
	}
	return p
}

func buildMemoryPanel(m systats.Memory) GaugePanel {
	return GaugePanel{
		OK:      true,
		Present: true,
		Gauge:   newGauge(m.PercentageUsed, fmt.Sprintf("%.0f%%", m.PercentageUsed), "Memory"),
		Used:    size(m.Used, m.Unit),
		Total:   size(m.Total, m.Unit),
		Free:    size(m.Free, m.Unit),
		Detail:  fmt.Sprintf("%s available", size(m.Available, m.Unit)),
	}
}

func buildSwapPanel(s systats.Swap) GaugePanel {
	// A host with swap off reports Total 0; a donut of 0/0 is noise, so
	// the template renders a muted "not configured" card instead.
	if s.Total <= 0 {
		return GaugePanel{OK: true, Present: false, Detail: "not configured"}
	}
	return GaugePanel{
		OK:      true,
		Present: true,
		Gauge:   newGauge(s.PercentageUsed, fmt.Sprintf("%.0f%%", s.PercentageUsed), "Swap"),
		Used:    size(s.Used, s.Unit),
		Total:   size(s.Total, s.Unit),
		Free:    size(s.Free, s.Unit),
		Detail:  fmt.Sprintf("%s free", size(s.Free, s.Unit)),
	}
}

func buildContainerPanel(cgMem systats.Memory, cgCPU systats.CPU, hostMem systats.Memory, hostCPU systats.CPU, unit systats.Unit) *ContainerPanel {
	p := &ContainerPanel{
		MemoryLimited: cgMem.Limited,
		CPULimited:    cgCPU.Limited,
		HostCores:     hostCPU.NoOfCores,
	}
	if cgMem.Limited {
		p.MemoryLimit = size(cgMem.Total, cgMem.Unit)
		p.MemoryHost = size(hostMem.Total, hostMem.Unit)
		p.MemoryGauge = newGauge(cgMem.PercentageUsed, fmt.Sprintf("%.0f%%", cgMem.PercentageUsed), "of limit")
	}
	if cgCPU.Limited {
		p.AllocatedCores = fmt.Sprintf("%.2f", cgCPU.AllocatedCores)
		p.CPUGauge = newGauge(float64(cgCPU.LoadAvg), fmt.Sprintf("%d%%", cgCPU.LoadAvg), "of quota")
	}
	return p
}

func buildPressurePanel(p systats.Pressure) PressurePanel {
	if !p.Available {
		return PressurePanel{
			Available: false,
			Note:      "kernel does not expose /proc/pressure - needs 4.20+ with CONFIG_PSI=y",
		}
	}

	panel := PressurePanel{Available: true, Limited: p.Limited}
	for _, r := range []struct {
		name string
		rp   systats.ResourcePressure
	}{
		{"CPU", p.CPU},
		{"Memory", p.Memory},
		{"I/O", p.IO},
	} {
		row := PressureRow{
			Name:          r.name,
			FullAvailable: r.rp.FullAvailable,
			Some:          pressureBars(r.rp.Some),
			SomeTotal:     stallTotal(r.rp.Some.Total),
		}
		if r.rp.FullAvailable {
			row.Full = pressureBars(r.rp.Full)
		}
		panel.Resources = append(panel.Resources, row)
	}
	return panel
}

func pressureBars(m systats.PressureMetric) []Bar {
	return []Bar{
		newBar(m.Avg10, "10s", fmt.Sprintf("%.2f%%", m.Avg10)),
		newBar(m.Avg60, "60s", fmt.Sprintf("%.2f%%", m.Avg60)),
		newBar(m.Avg300, "300s", fmt.Sprintf("%.2f%%", m.Avg300)),
	}
}

// stallTotal renders the cumulative stall counter, which the kernel keeps
// in microseconds.
func stallTotal(micros uint64) string {
	return (time.Duration(micros) * time.Microsecond).Round(time.Millisecond).String()
}

func buildDiskPanels(disks []systats.Disk, unit systats.Unit) []DiskPanel {
	out := make([]DiskPanel, 0, len(disks))
	for i := range disks {
		d := disks[i]
		pct := 0.0
		if d.Usage.Size > 0 {
			pct = 100 * d.Usage.Used / d.Usage.Size
		}

		// Convert mutates in place and returns an error for an unknown
		// unit. Ranging gives copies, so indexing would be required to
		// mutate the caller's slice - here the copy is what we want.
		if err := d.Convert(unit); err != nil {
			continue
		}

		out = append(out, DiskPanel{
			Device:     d.FileSystem,
			MountedOn:  d.MountedOn,
			Type:       d.Type,
			UsedPct:    pct,
			Bar:        newBar(pct, d.MountedOn, d.Usage.Usage),
			Used:       size(d.Usage.Used, d.Usage.Unit),
			Size:       size(d.Usage.Size, d.Usage.Unit),
			Available:  size(d.Usage.Available, d.Usage.Unit),
			InodePct:   d.Inodes.Usage,
			InodeUsage: fmt.Sprintf("%d / %d", d.Inodes.Used, d.Inodes.Inodes),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UsedPct > out[j].UsedPct })
	return out
}

// sampleDiskIO shows the documented way to turn /proc/diskstats into
// rates: the counters are cumulative since boot, so poll twice and let
// RatesSince do the arithmetic over the real elapsed time.
func sampleDiskIO(ctx context.Context, s *systats.SyStats, interval time.Duration) ([]DiskIOPanel, error) {
	before, err := s.GetDiskIO()
	if err != nil {
		return nil, err
	}

	start := time.Now()
	if err := sleepCtx(ctx, interval); err != nil {
		return nil, err
	}
	elapsed := time.Since(start).Seconds()

	after, err := s.GetDiskIO()
	if err != nil {
		return nil, err
	}

	prev := make(map[string]systats.DiskIO, len(before))
	for _, d := range before {
		prev[d.Device] = d
	}

	out := make([]DiskIOPanel, 0, len(after))
	for _, d := range after {
		p, ok := prev[d.Device]
		if !ok {
			continue // device appeared between samples
		}
		r := d.RatesSince(p, elapsed)
		out = append(out, DiskIOPanel{
			Device:    d.Device,
			ReadRate:  rate(r.ReadBytesPerSec),
			WriteRate: rate(r.WriteBytesPerSec),
			IOPS:      fmt.Sprintf("%.0f r/s, %.0f w/s", r.ReadsPerSec, r.WritesPerSec),
			Util:      newBar(r.UtilPercent, d.Device, fmt.Sprintf("%.1f%%", r.UtilPercent)),
			InFlight:  d.IOInProgress,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Device < out[j].Device })
	return out, nil
}

// sampleNetworks does for interfaces what RatesSince does for disks.
// There is no built-in helper here because Rx/Tx are plain counters on
// NetworkUsage, so the subtraction is the caller's job.
func sampleNetworks(ctx context.Context, s *systats.SyStats, interval time.Duration) ([]NetworkPanel, error) {
	before, err := s.GetNetworks()
	if err != nil {
		return nil, err
	}

	start := time.Now()
	if err := sleepCtx(ctx, interval); err != nil {
		return nil, err
	}
	elapsed := time.Since(start).Seconds()

	prev := make(map[string]systats.NetworkUsage, len(before))
	for _, n := range before {
		prev[n.Interface] = n.Usage
	}

	after, err := s.GetNetworks()
	if err != nil {
		return nil, err
	}

	out := make([]NetworkPanel, 0, len(after))
	for _, n := range after {
		panel := NetworkPanel{
			Interface: n.Interface,
			State:     n.Usage.State,
			Up:        n.Usage.State == "up",
			IP:        n.Ip,
			IPv6:      n.Ipv6,
			MAC:       n.MacAddress,
			RxTotal:   bytesHuman(float64(n.Usage.RxBytes)),
			TxTotal:   bytesHuman(float64(n.Usage.TxBytes)),
			Packets:   fmt.Sprintf("%d rx / %d tx", n.Usage.RxPackets, n.Usage.TxPackets),
			RxRate:    "-",
			TxRate:    "-",
		}
		if p, ok := prev[n.Interface]; ok && elapsed > 0 {
			panel.RxRate = rate(deltaPerSec(n.Usage.RxBytes, p.RxBytes, elapsed))
			panel.TxRate = rate(deltaPerSec(n.Usage.TxBytes, p.TxBytes, elapsed))
		}
		out = append(out, panel)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Interface < out[j].Interface })
	return out, nil
}

// sampleContainers reads every container on the host twice, interval
// apart, and turns the cumulative network and block I/O counters into
// rates with RatesSince. CPU needs no second pass: GetContainers already
// samples it over CPUSampleWindow, for all containers at once.
func sampleContainers(ctx context.Context, s *systats.SyStats, unit systats.Unit, interval time.Duration) ([]ContainerCard, string, error) {
	// Only the second snapshot's layer sizes are shown, so the first one
	// skips the file walk. A copy rather than toggling s: s is shared with
	// other goroutines, and flipping one of its fields would be a data race.
	quick := *s
	quick.ContainerLayerSize = false
	before, err := quick.GetContainersWithContext(ctx, unit)
	if err != nil {
		return nil, "", err
	}

	start := time.Now()
	if err := sleepCtx(ctx, interval); err != nil {
		return nil, "", err
	}

	after, err := s.GetContainersWithContext(ctx, unit)
	if err != nil {
		return nil, "", err
	}
	elapsed := time.Since(start).Seconds()

	prev := make(map[string]systats.Container, len(before))
	for _, c := range before {
		prev[c.ID] = c
	}

	cards := make([]ContainerCard, 0, len(after))
	named := 0
	for _, c := range after {
		var rates *systats.ContainerRates
		if p, ok := prev[c.ID]; ok {
			r := c.RatesSince(p, elapsed)
			rates = &r
		}
		if c.MetadataAvailable {
			named++
		}
		cards = append(cards, buildContainerCard(c, rates))
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Name < cards[j].Name })

	note := ""
	switch {
	case len(cards) == 0 || named > 0:
	case s.ContainerSocketPath == "":
		note = "No container runtime socket configured, so containers are listed by ID. Stats come from the cgroup tree either way."
	default:
		note = "No container runtime socket answered at " + s.ContainerSocketPath +
			", so containers are listed by ID. Stats come from the cgroup tree either way."
	}
	return cards, note, nil
}

func buildContainerCard(c systats.Container, rates *systats.ContainerRates) ContainerCard {
	card := ContainerCard{
		Name:    c.Name,
		ShortID: c.ShortID,
		Image:   c.Image,
		Runtime: c.Runtime,
		State:   c.State,
		Running: c.State == "running",
		RxRate:  "-", TxRate: "-", Read: "-", Write: "-",
	}
	if card.Name == "" {
		card.Name = c.ShortID
	}

	if c.CPU.Limited {
		card.CPUGauge = newGauge(c.CPU.PercentOfLimit, fmt.Sprintf("%.0f%%", c.CPU.PercentOfLimit), "of limit")
		card.CPU = fmt.Sprintf("%.2f of %.2f cores", c.CPU.CoresUsed, c.CPU.AllocatedCores)
	} else {
		card.CPUGauge = newGauge(c.CPU.PercentOfHost, fmt.Sprintf("%.0f%%", c.CPU.PercentOfHost), "of host")
		card.CPU = fmt.Sprintf("%.2f cores, no limit", c.CPU.CoresUsed)
	}

	m := c.Memory
	caption := "of host"
	if m.Limited {
		caption = "of limit"
	}
	card.MemGauge = newGauge(m.PercentageUsed, fmt.Sprintf("%.0f%%", m.PercentageUsed), caption)
	card.Memory = fmt.Sprintf("%s of %s", size(m.Used, m.Unit), size(m.Limit, m.Unit))

	if rates != nil {
		if c.Network.Accessible {
			card.RxRate = rate(rates.RxBytesPerSec)
			card.TxRate = rate(rates.TxBytesPerSec)
		}
		card.Read = rate(rates.ReadBytesPerSec)
		card.Write = rate(rates.WriteBytesPerSec)
	}

	if c.Pids.Limited {
		pct := 100 * float64(c.Pids.Current) / float64(c.Pids.Max)
		card.Pids = newBar(pct, "pids", fmt.Sprintf("%d / %d", c.Pids.Current, c.Pids.Max))
	} else {
		card.Pids = newBar(0, "pids", fmt.Sprintf("%d", c.Pids.Current))
	}

	if c.CPU.Periods > 0 {
		card.Throttled = fmt.Sprintf("%.1f%% of periods", 100*float64(c.CPU.ThrottledPeriods)/float64(c.CPU.Periods))
	}
	card.OOMKills = c.Memory.OOMKills

	switch {
	case c.Layer.Available:
		card.Layer = fmt.Sprintf("%s in %d files", size(c.Layer.Size, c.Layer.Unit), c.Layer.Files)
	case c.Layer.Path != "":
		card.Layer = "needs root"
	}

	for _, mt := range c.Mounts {
		row := ContainerMountRow{MountPoint: mt.MountPoint, Usage: "no access"}
		// The root overlay reports the disk holding every container's
		// layers - the same figures on every card. Say so, so it isn't
		// read as this container's usage; that's Layer above.
		if mt.MountPoint == "/" && mt.FSType == "overlay" {
			row.MountPoint = "/ (host disk)"
		}
		if mt.Accessible && mt.Total > 0 {
			pct := 100 * mt.Used / mt.Total
			row.Bar = newBar(pct, mt.MountPoint, fmt.Sprintf("%.0f%%", pct))
			row.Usage = fmt.Sprintf("%s of %s", size(mt.Used, mt.Unit), size(mt.Total, mt.Unit))
		}
		card.Mounts = append(card.Mounts, row)
	}

	if c.Network.SharesHostNetwork {
		card.Notes = append(card.Notes, "host network: rates are the host's")
	}
	if !c.Network.Accessible {
		card.Notes = append(card.Notes, "network namespace not readable")
	}
	if c.PodUID != "" {
		card.Notes = append(card.Notes, "pod "+c.PodUID)
	}
	return card
}

// deltaPerSec guards the counter-reset case: these are monotonic until an
// interface is reconfigured, at which point a naive subtraction would
// underflow uint64 into an enormous rate.
func deltaPerSec(now, then uint64, seconds float64) float64 {
	if now < then || seconds <= 0 {
		return 0
	}
	return float64(now-then) / seconds
}

func buildTCPPanel(s systats.TCPStates) TCPPanel {
	panel := TCPPanel{OK: true, Total: s.Total}

	buckets := []struct {
		label string
		count int
		color string
	}{
		{"Established", s.Established, colorEstablished},
		{"Listen", s.Listen, colorListen},
		{"Time wait", s.TimeWait, colorTimeWait},
		{"Close wait", s.CloseWait, colorCloseWait},
		{"Syn sent", s.SynSent, colorOther},
		{"Syn recv", s.SynRecv, colorOther},
		{"Fin wait 1", s.FinWait1, colorOther},
		{"Fin wait 2", s.FinWait2, colorOther},
		{"Closing", s.Closing, colorOther},
		{"Last ack", s.LastAck, colorOther},
		{"Close", s.Close, colorOther},
		{"New syn recv", s.NewSynRecv, colorOther},
		{"Unknown", s.Unknown, colorUnknown},
	}

	for _, b := range buckets {
		if b.count == 0 {
			continue
		}
		pct := 0.0
		if s.Total > 0 {
			pct = 100 * float64(b.count) / float64(s.Total)
		}
		panel.Segments = append(panel.Segments, Segment{
			Label:   b.label,
			Percent: pct,
			Width:   fmtFloat(pct),
			Color:   b.color,
		})
		panel.Rows = append(panel.Rows, LabelValue{
			Label: b.label,
			Value: fmt.Sprintf("%d", b.count),
		})
	}
	return panel
}

func buildTempPanels(temps []systats.Temperature) []TempPanel {
	out := make([]TempPanel, 0, len(temps))
	for _, t := range temps {
		pct, limits := tempScale(t)
		panel := TempPanel{
			Chip:     t.Name,
			Label:    t.Label,
			Celsius:  fmt.Sprintf("%.1f", t.Celsius),
			Gauge:    newGaugeColored(pct, fmt.Sprintf("%.0f°", t.Celsius), t.Label, tempColor(t)),
			Limits:   limits,
			Critical: t.CriticalAvailable && t.Celsius >= t.Critical,
		}
		out = append(out, panel)
	}
	return out
}

func buildProcessRows(procs []systats.Process) []ProcessRow {
	out := make([]ProcessRow, 0, len(procs))
	for _, p := range procs {
		row := ProcessRow{
			Pid:      p.Pid,
			Name:     p.Name,
			User:     p.User,
			State:    p.StateName,
			Threads:  p.Threads,
			CPU:      fmt.Sprintf("%.1f%%", p.CPUUsage),
			CPUBar:   newBar(float64(p.CPUUsage), "cpu", ""),
			Mem:      fmt.Sprintf("%.1f%%", p.MemUsage),
			MemBar:   newBar(float64(p.MemUsage), "mem", ""),
			ExecPath: p.ExecPath,
			// Reading another user's fd directory needs matching ownership
			// or root, so the count is only meaningful when accessible.
			OpenFDs: "n/a",
		}
		if p.FDsAccessible {
			row.OpenFDs = fmt.Sprintf("%d", p.OpenFDs)
		}
		out = append(out, row)
	}
	return out
}

// sleepCtx waits for d or returns early if ctx is cancelled. In -serve
// mode this is what lets a closed browser tab abort an in-flight sample
// instead of holding the goroutine for the full interval.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
