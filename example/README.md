# systats dashboard example

A single-page system dashboard built on
[systats](https://github.com/dhamith93/systats). Run it, get an HTML file.

```bash
go run ./example                      # writes example/dashboard.html
go run ./example -unit GB -top 15
go run ./example -serve :8080         # re-collects on every request
```

The generated page has **no `<script>` tags, no CDN references and no
external assets**. Every gauge is inline SVG whose geometry is computed in
Go. That means you can `scp` the file off a server and open it on a
machine with no network.

## Flags

| Flag | Default | What it does |
|---|---|---|
| `-out` | `example/dashboard.html` | Where to write the snapshot |
| `-serve` | — | Serve on this address instead of writing a file |
| `-refresh` | `5s` | Auto-refresh interval in serve mode |
| `-unit` | `MB` | Size unit: `B`, `KB`, `MB`, `GB` (all binary) |
| `-sample-window` | `300ms` | How long `GetCPU` samples for |
| `-sample-interval` | `1s` | Gap between the two reads used for disk/network rates |
| `-top` | `8` | How many processes to list |
| `-timeout` | `30s` | Overall deadline for one collection |
| `-container-socket` | `/var/run/docker.sock` | Docker-compatible API socket used to name containers |

## Panels

CPU utilisation and per-core bars, memory and swap, pressure stall
information, filesystems, disk I/O rates, network interfaces, TCP socket
states, temperatures, and the top processes by CPU. A container banner
appears above everything when a cgroup limit is detected.

When run on a host with containers, a "Containers on this host" section
shows one card per container: CPU and memory gauges against its limits,
network and disk rates, pids, throttling, OOM kills and mount usage. Mount
usage needs root; the rest works unprivileged.

## Reading it as an example

[`collect.go`](collect.go) is the part worth copying. It shows:

- **`ContainerAware`** — the host-wide and container-aware readings are
  collected from **two separate `SyStats` values**, not one with the flag
  toggled. `ContainerAware` is an ordinary mutable field, so flipping a
  shared instance while another goroutine is mid-call is a data race.
- **Concurrency** — the independent readers run in parallel goroutines. A
  `SyStats` is safe to share once configured, because no method writes to
  its receiver.
- **`RatesSince`** — `/proc/diskstats` counters are cumulative since boot,
  so the dashboard polls twice and diffs. `sampleNetworks` does the same
  thing by hand for Rx/Tx, which have no built-in helper.
- **Degradation** — every panel that can be absent has a flag
  (`Pressure.Available`, `Temperature.HighAvailable`,
  `Process.FDsAccessible`, …), and a failed panel becomes a row in
  "Unavailable on this host" rather than aborting the page. Running this
  on macOS is a quick way to see that path: almost everything fails and
  you still get a complete dashboard.
- **Contexts** — in `-serve` mode the handler passes `r.Context()` down,
  so closing the browser tab aborts the in-flight CPU sample instead of
  leaving a goroutine to wait out the window.
- **`ProcessCPUMode`** — the process table uses `CPUUsageAverage`, a
  single `/proc` read per process. The default `CPUUsageInstant` costs a
  second sampling window; on real hardware that is ~320ms versus ~6ms for
  the same table.

[`render.go`](render.go) holds the SVG geometry. `html/template` can't do
arithmetic, so percentages, arc lengths and colours are all resolved in Go
before the template runs.

## On a remote host

The template is embedded with `go:embed`, so the cross-compiled binary is
the only file you need:

```bash
make example-linux
scp example/systats-dashboard-linux user@host:
ssh user@host './systats-dashboard-linux -out dashboard.html'
scp user@host:dashboard.html .
```
