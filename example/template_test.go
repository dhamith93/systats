package main

import (
	"strings"
	"testing"
	"time"
)

// fullDashboard is a synthetic, fully-populated Dashboard. It exists
// because the interesting template paths - gauges, the container banner,
// pressure rows, temperature tiles - are all unreachable on a non-Linux
// developer machine, where collect() fills Problems instead. Without
// this, a mistyped field reference would only fail on a real Linux host.
func fullDashboard() Dashboard {
	bars := []Bar{
		newBar(1.23, "10s", "1.23%"),
		newBar(0.45, "60s", "0.45%"),
		newBar(0.10, "300s", "0.10%"),
	}

	return Dashboard{
		GeneratedAt: "Mon 22 Sep 2026 19:57:00 UTC",
		Elapsed:     "1.3s",
		Host: HostPanel{
			OK: true, Hostname: "athena", OS: "Debian GNU/Linux 12",
			Kernel: "6.1.0-18-amd64", Uptime: "72h3m0s", LoggedInUsers: 2,
		},
		CPU: CPUPanel{
			OK:    true,
			Gauge: newGauge(37, "37%", "CPU"),
			Cores: []Bar{
				newBar(41, "cpu0", "41%"),
				newBar(33, "cpu1", "33%"),
			},
			Sockets: 1, Model: "Intel Core i7-8700",
			CoresLabel: "8 logical / 4 physical",
			Load1:      "0.63", Load5: "0.18", Load15: "0.06",
		},
		Memory: GaugePanel{
			OK: true, Present: true,
			Gauge: newGauge(53.6, "54%", "Memory"),
			Used:  "8534.86 MB", Total: "15932.95 MB", Free: "3739.51 MB",
			Detail: "7830.01 MB available",
		},
		Swap: GaugePanel{
			OK: true, Present: true,
			Gauge: newGauge(3.8, "4%", "Swap"),
			Used:  "78.12 MB", Total: "2047.99 MB", Free: "1969.87 MB",
		},
		Container: &ContainerPanel{
			MemoryLimited: true, MemoryLimit: "256.00 MB", MemoryHost: "3916.22 MB",
			MemoryGauge: newGauge(64, "64%", "of limit"),
			CPULimited:  true, AllocatedCores: "0.50",
			CPUGauge:  newGauge(48, "48%", "of quota"),
			HostCores: 2,
		},
		Containers: []ContainerCard{{
			Name: "web", ShortID: "0123456789ab", Image: "nginx:1.27", Runtime: "docker",
			State: "running", Running: true,
			CPUGauge: newGauge(84, "84%", "of limit"), CPU: "0.42 of 0.50 cores",
			MemGauge: newGauge(35, "35%", "of limit"), Memory: "90.00 MB of 256.00 MB",
			RxRate: "12.00 KB/s", TxRate: "3.00 KB/s", Read: "idle", Write: "1.50 MB/s",
			Pids:      newBar(17, "pids", "17 / 100"),
			Throttled: "25.0% of periods", OOMKills: 2,
			Mounts: []ContainerMountRow{
				{MountPoint: "/", Bar: newBar(41, "/", "41%"), Usage: "41000.00 MB of 100000.00 MB"},
				{MountPoint: "/var/lib/data", Usage: "no access"},
			},
			Notes: []string{"host network: rates are the host's"},
		}},
		ContainersNote: "No container runtime socket answered",
		Pressure: PressurePanel{
			Available: true, Limited: true,
			Resources: []PressureRow{
				{Name: "CPU", Some: bars, Full: bars, FullAvailable: true, SomeTotal: "1.2s"},
				// Deliberately mixed: /proc/pressure/cpu has no full line
				// on most kernels, so this branch has to render too.
				{Name: "Memory", Some: bars, FullAvailable: false, SomeTotal: "0s"},
				{Name: "I/O", Some: bars, Full: bars, FullAvailable: true, SomeTotal: "42ms"},
			},
		},
		Disks: []DiskPanel{{
			Device: "/dev/sda2", MountedOn: "/", Type: "ext4",
			UsedPct: 97, Bar: newBar(97, "/", "97%"),
			Used: "102607.84 MB", Size: "112162.13 MB", Available: "3814.69 MB",
			InodePct: "12%", InodeUsage: "812345 / 6553600",
		}},
		DiskIO: []DiskIOPanel{{
			Device: "sda", ReadRate: "1.20 MB/s", WriteRate: "idle",
			IOPS: "12 r/s, 0 w/s", Util: newBar(8.4, "sda", "8.4%"), InFlight: 0,
		}},
		Networks: []NetworkPanel{{
			Interface: "eth0", State: "up", Up: true,
			IP: "192.168.1.50", IPv6: "fe80::1", MAC: "aa:bb:cc:dd:ee:ff",
			RxTotal: "1.15 GB", TxTotal: "942.00 MB",
			RxRate: "24.00 KB/s", TxRate: "idle", Packets: "4500123 rx / 3200456 tx",
		}},
		TCP: TCPPanel{
			OK: true, Total: 12,
			Segments: []Segment{
				{Label: "Established", Percent: 33.3, Width: "33.3", Color: colorEstablished},
				{Label: "Listen", Percent: 66.7, Width: "66.7", Color: colorListen},
			},
			Rows: []LabelValue{{Label: "Established", Value: "4"}, {Label: "Listen", Value: "8"}},
		},
		Temps: []TempPanel{{
			Chip: "coretemp", Label: "Package id 0", Celsius: "43.0",
			Gauge:  newGauge(43, "43°", "Package id 0"),
			Limits: "high 80°C, crit 100°C",
		}},
		Processes: []ProcessRow{{
			Pid: 1234, Name: "postgres", User: "postgres", State: "sleeping",
			Threads: 7, CPU: "12.4%", CPUBar: newBar(12.4, "cpu", ""),
			Mem: "3.1%", MemBar: newBar(3.1, "mem", ""),
			OpenFDs: "64", ExecPath: "/usr/lib/postgresql/15/bin/postgres -D /var/lib/postgresql",
		}},
		Problems: []Problem{{Panel: "Temperatures", Err: "no hwmon chips"}},
	}
}

func TestRenderFullDashboard(t *testing.T) {
	var sb strings.Builder
	if err := render(&sb, fullDashboard(), 0); err != nil {
		t.Fatalf("render returned %v", err)
	}
	out := sb.String()

	// Every panel must actually appear. A mistyped field reference in the
	// template renders as empty rather than erroring, so assert on
	// content, not just on the absence of an error.
	for _, want := range []string{
		"athena", "Debian GNU/Linux 12", "6.1.0-18-amd64", "72h3m0s",
		"0.63", "8 logical / 4 physical", "Intel Core i7-8700",
		"8534.86 MB", "15932.95 MB",
		"Container limits detected", "256.00 MB", "0.50",
		"Pressure stall information", "1.23%",
		"/dev/sda2", "112162.13 MB",
		"1.20 MB/s", "eth0", "192.168.1.50",
		"Established", "coretemp", "Package id 0",
		"postgres", "12.4%",
		"Containers on this host", "nginx:1.27", "0.42 of 0.50 cores", "90.00 MB of 256.00 MB",
		"1.50 MB/s", "17 / 100", "25.0% of periods", "2 OOM kills", "/var/lib/data", "no access",
		"host network: rates are the host&#39;s", "No container runtime socket answered",
		"Unavailable on this host", "no hwmon chips",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}

	// The resource with FullAvailable false must say so rather than
	// silently showing a row of zeros.
	if !strings.Contains(out, "No <code>full</code> line") {
		t.Errorf("expected a note for the resource with no full line")
	}
}

// The page has to stay self-contained: no scripts and no external assets,
// so it can be copied off a server and opened offline.
func TestRenderedPageHasNoExternalAssets(t *testing.T) {
	var sb strings.Builder
	if err := render(&sb, fullDashboard(), 0); err != nil {
		t.Fatalf("render returned %v", err)
	}
	out := sb.String()

	for _, forbidden := range []string{"<script", "cdnjs", "unpkg", "googleapis", "src=\"http"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("rendered page contains %q, breaking self-containment", forbidden)
		}
	}
}

func TestRenderRefreshMeta(t *testing.T) {
	var sb strings.Builder
	if err := render(&sb, fullDashboard(), 5*time.Second); err != nil {
		t.Fatalf("render returned %v", err)
	}
	if !strings.Contains(sb.String(), `http-equiv="refresh" content="5"`) {
		t.Errorf("expected a 5 second meta refresh")
	}

	// A snapshot written to a file must not reload itself - the data is
	// frozen at write time, so refreshing would only flicker.
	sb.Reset()
	if err := render(&sb, fullDashboard(), 0); err != nil {
		t.Fatalf("render returned %v", err)
	}
	if strings.Contains(sb.String(), "http-equiv=\"refresh\"") {
		t.Errorf("snapshot mode must not emit a meta refresh")
	}
}

// A sub-second interval would floor to 0, which browsers read as "reload
// immediately, forever".
func TestRenderRefreshNeverZero(t *testing.T) {
	var sb strings.Builder
	if err := render(&sb, fullDashboard(), 200*time.Millisecond); err != nil {
		t.Fatalf("render returned %v", err)
	}
	if !strings.Contains(sb.String(), `content="1"`) {
		t.Errorf("a sub-second refresh should round up to 1s, not 0")
	}
}

// An empty Dashboard is what a host where everything failed produces. It
// must still render a complete page rather than panicking on a nil field.
func TestRenderEmptyDashboard(t *testing.T) {
	var sb strings.Builder
	if err := render(&sb, Dashboard{}, 0); err != nil {
		t.Fatalf("render returned %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "systats dashboard") {
		t.Errorf("expected the fallback title when no hostname was collected")
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "</html>") {
		t.Errorf("page is truncated; want a complete document")
	}
}
