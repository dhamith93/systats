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
package systats
