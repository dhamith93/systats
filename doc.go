// Package systats collects Linux system statistics: CPU, memory, swap,
// disks, network interfaces, running processes, and service status.
// It reads directly from /proc and /sys wherever possible instead of
// shelling out to external binaries.
//
// Basic usage:
//
//	syStats := systats.New()
//	memory, err := syStats.GetMemory(systats.Megabyte)
//
// This package is Linux-only - most Get* methods depend on /proc, /sys,
// or systemd/SysV service tooling that don't exist on other platforms.
//
// # Contexts
//
// Methods that can block have a WithContext variant:
//
//	cpu, err := syStats.GetCPUWithContext(ctx)
//
// These are GetCPU, GetTopProcesses, GetProcess, GetSystem,
// IsServiceRunning, CanConnectExternal and IsPortOpen - the ones that
// sample over a time window, shell out, or touch the network. The
// non-context forms remain, and simply pass context.Background().
//
// The remaining methods deliberately have no context variant. They only
// read local files under /proc and /sys, and a read that has already begun
// cannot be interrupted in Go - so a ctx parameter there would advertise a
// cancellation the package could not actually perform.
package systats
