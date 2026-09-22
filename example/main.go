// Command example renders a single-page system dashboard using
// github.com/dhamith93/systats.
//
// By default it writes a self-contained HTML file and exits:
//
//	go run ./example
//	go run ./example -out /tmp/dashboard.html -unit GB
//
// With -serve it stays up and re-collects on every request, so the
// numbers move:
//
//	go run ./example -serve :8080
//
// The generated page has no <script>, no CDN references and no external
// assets - every gauge is inline SVG computed in Go. That means the file
// can be copied off a server and opened anywhere, including on a machine
// with no network.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhamith93/systats"
)

// Config is the knobs the dashboard collection takes.
type Config struct {
	Unit systats.Unit
	// SampleWindow is how long GetCPU spends measuring. It is the single
	// biggest cost in a collection, and the library makes it adjustable.
	SampleWindow time.Duration
	// SampleInterval separates the two reads used to turn cumulative
	// disk and network counters into rates. Longer is steadier.
	SampleInterval time.Duration
	TopProcesses   int
}

func main() {
	var (
		out            = flag.String("out", "example/dashboard.html", "where to write the HTML dashboard")
		serve          = flag.String("serve", "", "serve on this address instead of writing a file (e.g. :8080)")
		refresh        = flag.Duration("refresh", 5*time.Second, "auto-refresh interval in -serve mode")
		unit           = flag.String("unit", "MB", "size unit: B, KB, MB or GB")
		sampleWindow   = flag.Duration("sample-window", 300*time.Millisecond, "CPU sampling window")
		sampleInterval = flag.Duration("sample-interval", time.Second, "interval between the two reads used for disk and network rates")
		top            = flag.Int("top", 8, "how many processes to list")
		timeout        = flag.Duration("timeout", 30*time.Second, "overall deadline for one collection")
	)
	flag.Parse()

	u, err := parseUnit(*unit)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	cfg := Config{
		Unit:           u,
		SampleWindow:   *sampleWindow,
		SampleInterval: *sampleInterval,
		TopProcesses:   *top,
	}

	if *serve != "" {
		if err := serveDashboard(*serve, *refresh, *timeout, cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	data := collect(ctx, cfg)
	path, err := writeSnapshot(*out, data, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to write dashboard:", err)
		os.Exit(1)
	}

	summarize(data)
	fmt.Println("\nDashboard written to:", path)
}

// parseUnit keeps the flag stringly-typed at the boundary while the rest
// of the program uses systats.Unit. The library validates too, but
// failing here gives a better message than a collection error per panel.
func parseUnit(s string) (systats.Unit, error) {
	switch systats.Unit(strings.ToUpper(strings.TrimSpace(s))) {
	case systats.Byte:
		return systats.Byte, nil
	case systats.Kilobyte:
		return systats.Kilobyte, nil
	case systats.Megabyte:
		return systats.Megabyte, nil
	case systats.Gigabyte:
		return systats.Gigabyte, nil
	default:
		return "", fmt.Errorf("unsupported unit %q: want B, KB, MB or GB", s)
	}
}

// writeSnapshot renders to a file, creating the parent directory. That
// matters for the cross-compiled binary, which is usually scp'd somewhere
// and run outside the repo where example/ doesn't exist.
func writeSnapshot(out string, data Dashboard, refresh time.Duration) (string, error) {
	path, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err := render(f, data, refresh); err != nil {
		return "", err
	}
	return path, nil
}

func serveDashboard(addr string, refresh, timeout time.Duration, cfg Config) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		// r.Context() is cancelled when the client disconnects, so
		// passing it down means closing the tab actually aborts the
		// in-flight CPU sample instead of leaving a goroutine to wait
		// out the window. This is what the WithContext variants are for.
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		data := collect(ctx, cfg)

		if err := ctx.Err(); err != nil {
			// Client went away mid-collection; nothing to write to.
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := render(w, data, refresh); err != nil {
			log.Println("render:", err)
		}
	})

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// Generous relative to the read timeout: one collection includes
		// the CPU sampling window plus the rate-sampling interval.
		WriteTimeout: timeout + 10*time.Second,
		ReadTimeout:  10 * time.Second,
	}

	fmt.Printf("Serving on http://%s (refreshing every %s, Ctrl-C to stop)\n", displayAddr(addr), refresh)
	return srv.ListenAndServe()
}

func displayAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}

// summarize prints a short console digest so the command is useful over
// ssh without opening the file.
func summarize(d Dashboard) {
	fmt.Printf("%-14s %s\n", "Host", d.Host.Hostname)
	fmt.Printf("%-14s %s\n", "OS", d.Host.OS)
	fmt.Printf("%-14s %s\n", "Uptime", d.Host.Uptime)
	if d.CPU.OK {
		fmt.Printf("%-14s %s across %d cores\n", "CPU", d.CPU.Gauge.Label, len(d.CPU.Cores))
	}
	if d.Memory.OK {
		fmt.Printf("%-14s %s of %s used\n", "Memory", d.Memory.Used, d.Memory.Total)
	}
	if d.Container != nil {
		if d.Container.MemoryLimited {
			fmt.Printf("%-14s memory limited to %s\n", "Container", d.Container.MemoryLimit)
		}
		if d.Container.CPULimited {
			fmt.Printf("%-14s %s cores allocated\n", "Container", d.Container.AllocatedCores)
		}
	}
	if d.Pressure.Available {
		for _, r := range d.Pressure.Resources {
			fmt.Printf("%-14s %s some avg10 %s\n", "Pressure", r.Name, r.Some[0].Value)
		}
	}
	fmt.Printf("%-14s %d sensors, %d disks, %d interfaces, %d processes\n",
		"Collected", len(d.Temps), len(d.Disks), len(d.Networks), len(d.Processes))
	fmt.Printf("%-14s %s\n", "Took", d.Elapsed)

	for _, p := range d.Problems {
		fmt.Printf("%-14s %s: %s\n", "Unavailable", p.Panel, p.Err)
	}
}
