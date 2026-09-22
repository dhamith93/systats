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
// # Concurrency
//
// A SyStats is safe for concurrent use once configured. No method writes
// to its receiver, so any number of goroutines may share one value:
//
//	syStats := systats.New()
//	syStats.ContainerAware = true   // configure first
//	go poll(&syStats)               // then share freely
//	go poll(&syStats)
//
// The configuration fields - the /proc and /sys paths, ContainerAware,
// ProcessCPUMode and CPUSampleWindow - are ordinary struct fields with no
// synchronization. Set them before the first call. Changing one while
// another goroutine is calling a method is a data race; give each
// goroutine its own SyStats if they need different settings.
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
