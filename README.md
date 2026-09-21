# systats

Go module to get linux system stats.

[![Go](https://github.com/dhamith93/systats/actions/workflows/go.yml/badge.svg)](https://github.com/dhamith93/systats/actions/workflows/go.yml)

Provides following information on systems:
* System
	* Returns OS, Hostname, Kernel, Up time, last boot date, timezone, logged in users list
* CPU
	* CPU model, freq, usage (overall, per core), load average (1/5/15 min), etc
* Memory/SWAP
	* Host-wide, or the calling process's cgroup limits (see [Container aware stats](#container-aware-stats))
* Disks
	* File system, type, mount point, usage, inodes
* Networks
	* Interface, IP, Rx/Tx
* Service status
	* Returns if given service is active or not
* Processes
	* Returns list of processes sorted by CPU/Memory usage (with PID, exec path, user, usage)

## Usage

Import the module 

```go
import (
	"github.com/dhamith93/systats"
)

func main() {
    syStats := systats.New()
}
```

And use the methods to get the required and supported system stats.

### System

Returns OS, Hostname, Kernel, Up time, last boot date, timezone, logged in users list

```go
func main() {
	syStats := systats.New()
	system, err := systats.GetSystem()
}
```

### CPU

CPU info and usage info (overall, and per core)

```go
func main() {
	syStats := systats.New()
	cpu, err := systats.GetCPU()
}
```

Two different metrics live on `CPU`, don't mix them up:

* `LoadAvg`/`CoreAvg` - CPU *utilization* as a percentage, sampled over a 300ms window.
* `Load1`/`Load5`/`Load15` - the traditional Unix load average from `/proc/loadavg`, what `uptime` shows. Not a percentage, and can exceed 100.

### Memory

```go
func main() {
	syStats := systats.New()
	memory, err := systats.GetMemory(systats.Megabyte)
}
```

### SWAP

```go
func main() {
	syStats := systats.New()
	swap, err := systats.GetSwap(systats.Megabyte)
}
```

### Disks

```go
func main() {
	syStats := systats.New()
	disks, err := syStats.GetDisks()
}
```

### Networks

Interface info and usage info

```go
func main() {
	syStats := systats.New()
	networks, err := syStats.GetNetworks()
}
```

### Service status

Returns if service is running or not

```go
func main() {
	syStats := systats.New()
	running := syStats.IsServiceRunning(service)
	if !running {
		fmt.Println(service + " not running")
	}
}
```

### Running processes

Returns running processes sorted by CPU or memory usage

```go
func main() {
	syStats := systats.New()
	procs, err := syStats.GetTopProcesses(10, "cpu")
	procs, err := syStats.GetTopProcesses(10, "memory")
}
```

`SyStats.ProcessCPUMode` controls how `CPUUsage` is calculated:

* `systats.CPUUsageInstant` (default): usage over a live 300ms sampling window. Accurate for current load, adds 300ms+ latency per call.
* `systats.CPUUsageAverage`: lifetime average (total CPU time / time since process start), same as `ps`'s `%cpu`. Single `/proc` read, no sampling delay.

```go
func main() {
	syStats := systats.New()
	syStats.ProcessCPUMode = systats.CPUUsageAverage
	procs, err := syStats.GetTopProcesses(10, "cpu")
}
```

### Container aware stats

`GetMemory` and `GetCPU` read host-wide `/proc` by default. Inside a container that's the wrong machine - a service capped at 512MB will report the host's full RAM as `Total`, right up until it gets OOM-killed.

Set `ContainerAware` to read the calling process's own cgroup (v1 and v2, auto-detected) instead:

```go
func main() {
	syStats := systats.New()
	syStats.ContainerAware = true

	mem, err := syStats.GetMemory(systats.Megabyte)
	// mem.Limited == true, mem.Total is the cgroup limit
	cpu, err := syStats.GetCPU()
	// cpu.Limited == true, cpu.LoadAvg is % of cpu.AllocatedCores
}
```

Notes:

* Defaults to off. When there's no cgroup or no limit configured, it silently falls back to the host-wide numbers - check `Memory.Limited`/`CPU.Limited` to tell which you got.
* `AllocatedCores` is the quota in cores and can be fractional (Kubernetes `500m` is `0.5`). `NoOfCores` always means host cores.
* This reports on its *own* cgroup, so it belongs inside the container it describes. It won't enumerate other containers on a host.
* Also applies to plain systemd units with `MemoryMax=`/`CPUQuota=` set, not just containers.
* `Load1`/`Load5`/`Load15` stay host-wide either way - there's no cgroup equivalent.