# systats

Go module to get linux system stats.

[![Go](https://github.com/dhamith93/systats/actions/workflows/go.yml/badge.svg)](https://github.com/dhamith93/systats/actions/workflows/go.yml)

Provides following information on systems:
* System
	* Returns OS, Hostname, Kernel, Up time, last boot date, timezone, logged in users list
* CPU
	* CPU model, freq, load average (overall, per core), etc
* Memory/SWAP
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

CPU info and load avg info (overall, and per core)

```go
func main() {
	syStats := systats.New()
	cpu, err := systats.GetCPU()
}
```

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