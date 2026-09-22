// Package systats collects Linux system statistics: CPU, memory, swap,
// disks, network interfaces, pressure stall information, temperatures,
// running processes, and service status.
//
// Basic usage:
//
//	syStats := systats.New()
//	memory, err := syStats.GetMemory(systats.Megabyte)
//
// This package is Linux-only - most Get* methods depend on /proc, /sys,
// or systemd/SysV service tooling that don't exist on other platforms.
//
// # Subprocesses
//
// No stats call spawns a subprocess: CPU, memory, swap, disk, disk I/O,
// network, pressure, temperature and process data all come from reading
// /proc and /sys directly. That means they work on a minimal image with
// no ps, df or ip installed.
//
// Two calls are the exception, and will fail on such an image:
//
//   - IsServiceRunning runs "systemctl is-active", falling back to
//     "service <name> status". Neither exists in a distroless container.
//   - GetSystem runs who(1) to fill in System.LoggedInUsers. The rest of
//     the System fields are read from /proc and /etc, so they are still
//     populated when who is missing - only the user list comes back
//     empty.
//
// Both honor the context passed to their WithContext variants, and are
// bounded by a 5s timeout otherwise.
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
