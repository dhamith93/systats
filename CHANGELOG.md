# Changelog

All notable changes to this project are documented here, following the
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/) format.

## [Unreleased]

### Changed
- **`DiskUsage` size fields are now `float64`** (`Size`, `Used`,
  `Available`), matching `Memory` and `Swap`. As integers, `Disk.Convert`
  truncated on every step: a 512 MiB partition converted to `Gigabyte`
  reported `0`, and a bytes -> `Gigabyte` -> bytes round trip on a 500 GB
  disk returned 499289948160 instead of 500107862016, silently losing
  818 MB. `InodeUsage` keeps integer fields - inodes are counts, not sizes.

### Added
- **`WithContext` variants for the seven methods that can block**:
  `GetCPUWithContext`, `GetTopProcessesWithContext`,
  `GetProcessWithContext`, `GetSystemWithContext`,
  `IsServiceRunningWithContext`, `CanConnectExternalWithContext` and
  `IsPortOpenWithContext`. These are the calls that sample over a time
  window, shell out, or touch the network; the plain forms remain and
  delegate with `context.Background()`. Previously `GetCPU` would hold a
  goroutine for its full 300ms window even after the caller had gone away.

  The other methods deliberately have no variant - they only read local
  files under `/proc` and `/sys`, and a read already in flight can't be
  interrupted in Go, so a `ctx` parameter would advertise a cancellation
  that couldn't happen.
- **`SyStats.CPUSampleWindow`** - how far apart the two CPU samples are
  taken, 300ms by default. It's the dominant cost of `GetCPU`, and of
  `GetTopProcesses`/`GetProcess` in `CPUUsageInstant` mode, so shortening
  it trades accuracy for latency. A context can abort a sample but can't
  shorten it, which is why this knob exists alongside. Zero falls back to
  the default, so a hand-constructed `SyStats{}` doesn't sample over no
  time at all.
- **`IsServiceRunningWithContext` returns `(bool, error)`**, unlike
  `IsServiceRunning`'s bare `bool`. A wedged `systemctl` (bounded at 5s) or
  a missing binary previously reported the service as *stopped*: `false,
  nil` now means genuinely stopped, `false, err` means the check failed.
- Context-aware variants in the `exec` package (`ExecuteWithContext`,
  `ExecuteWithErrorAndContext`, and the pipe equivalents). The 5s default
  timeout still applies as a backstop when the passed context has no
  deadline of its own.

### Fixed
- `isServiceRunning` now decides on the tool's *output* rather than its exit
  code. `systemctl is-active` exits non-zero for an inactive service just as
  it does for a genuine failure, so the exit code alone could not tell those
  apart; a tool that printed a verdict has answered the question regardless
  of how it exited.

## [v0.3.1] - 2026-09-22

### Changed
- **`Memory` and `Swap` size fields are now `float64`** (`Total`, `Used`,
  `Free`, `Available`). As integers the larger units were close to useless:
  4 GiB of RAM reported as `3 GB`, discarding 21% of the value, and any
  partition under 1 GiB reported `0`. `GetMemory(Gigabyte).Total` now reads
  e.g. `15.56` rather than `15`. `Disk` keeps integer fields - its values are
  large enough that truncation is negligible.
- **Memory and swap values in `Megabyte` and `Kilobyte` have changed**, because
  the conversions were wrong. All units are now consistently binary -
  `Kilobyte` is KiB, `Megabyte` is MiB, `Gigabyte` is GiB - matching what
  `free -m`, `df -h` and `top` report, and matching what `Disk.Convert`
  already did. Concretely, for a host with `MemTotal: 16315340 kB`,
  `GetMemory(Megabyte).Total` goes from `16315` to `15932` and
  `GetMemory(Kilobyte).Total` from `16706908` to `16315340`. A container run
  with `-m 512m` now correctly reports `512` rather than `524`.
- `GetTopProcesses` returns an error for an unrecognized sort order instead of
  silently sorting by CPU, so a typo like `"memroy"` is no longer a
  successful-looking call. An empty string still means CPU.

### Added
- `SortByCPU` and `SortByMemory` constants for `GetTopProcesses` - the sort
  order was previously an undocumented magic string.
- `GetMemory` and `GetSwap` now accept `Byte` and `Gigabyte`. Both constants
  were already exported but every call using them returned
  `"B is not supported"`.

### Fixed
- `internal/unitconv.KibToMB` mixed decimal and binary arithmetic (it
  multiplied by 1.024 to reach decimal KB, then divided by a binary 1024),
  making every megabyte figure roughly 2.4% wrong - 512 MiB reported as 524,
  which is neither 512 MiB nor 536 MB. `KibToGB` had the same fault and no
  callers. The package had no tests at all, which is how this survived; it's
  now at 100% coverage.
- The first four README examples called methods on the package name
  (`systats.GetSystem()` rather than `syStats.GetSystem()`) and so did not
  compile. Every example in the README is now compile-checked.

## [v0.3.0] - 2026-09-22

The headline changes are that `GetMemory`/`GetCPU` can now report a
container's own cgroup limits instead of the host's numbers, that no
external binary is shelled out for stats collection any more, and that
disk I/O, TCP connection states and protocol counters are now available.

Two changes need attention when upgrading: JSON field names are now
lowerCamelCase, and `CPU.NoOfCores` means logical CPUs rather than
physical cores. See **Changed** below.

### Removed
- Shelled-out `ps`, `df`, `ip`, `lsof`, and `whereis`/`GetExecPath` -
  replaced with `/proc`, `/sys`, `net.Interfaces()`, and
  `golang.org/x/sys/unix.Statfs`. No external process is spawned for
  `GetCPU`, `GetMemory`, `GetSwap`, `GetDisks`, `GetNetworks`,
  `GetTopProcesses`, or `EstablishedTCPConnCount` anymore.

### Added
- `GetProcess(pid)` - look up a single process instead of only the top N.
  Returns an error when the pid isn't readable, unlike `GetTopProcesses`
  which skips such processes. Honors `ProcessCPUMode`, so the default
  instant mode costs a ~300ms sampling window per call.
- `Process` gained `Name` (stat's `comm`, the only name a kernel thread
  has), `State`/`StateName`, `Threads`, `OpenFDs`/`FDsAccessible`, and
  `IO` (per-process counters from `/proc/<pid>/io`). These are populated
  by `GetTopProcesses` too. `IO` and `OpenFDs` are permission-gated -
  check their `Accessible` flags before reading zeros as real.
- `SyStats.ProcPath` - overridable `/proc` root, which is what makes the
  process code testable against a fixture tree instead of a live system.
- `GetTCPConnectionStates()` - counts every TCP socket in the network
  namespace by connection state (ESTABLISHED, TIME_WAIT, LISTEN, ...),
  IPv4 and IPv6 combined. Complements `EstablishedTCPConnCount`, which
  remains the per-process view. Overridable via `SyStats.NetTCPPath` and
  `SyStats.NetTCP6Path`.
- `GetProtocolStats()` - TCP/UDP/IP/ICMP counters merged from
  `/proc/net/snmp` and `/proc/net/netstat` (retransmits, listen
  overflows, UDP errors, ...). Keyed by protocol then counter name;
  `Value(protocol, counter)` returns `ok=false` for counters this kernel
  doesn't implement, which a fixed struct couldn't distinguish from
  zero. Overridable via `SyStats.NetSNMPPath` and
  `SyStats.NetNetstatPath`.
- `GetDiskIO()` - per-device I/O counters from `/proc/diskstats`
  (reads/writes completed and merged, bytes read/written, time spent,
  queue depth, plus discard and flush stats where the kernel provides
  them). Values are cumulative counters since boot; `DiskIO.RatesSince`
  turns two samples into throughput, IOPS and `iostat`-style `%util`.
  Overridable via `SyStats.DiskStatsPath`.
- `CPU.PhysicalCores` and `CPU.Sockets` - counted from distinct
  `(physical id, core id)` pairs in `/proc/cpuinfo`, which is correct on
  multi-socket and hybrid P+E-core machines where `cpu cores` is not.
  Falls back to the logical count on architectures that don't publish
  topology (ARM, RISC-V, some containers).
- `SyStats.ContainerAware` - opt-in (default false, so nothing changes
  for existing callers). When set, `GetMemory`/`GetCPU` report the
  calling process's own cgroup limits (v1 and v2, auto-detected) instead
  of host-wide `/proc` numbers, which are the wrong machine inside a
  container. Adds `Memory.Limited`, `CPU.Limited` and
  `CPU.AllocatedCores` (the quota in cores, fractional - Kubernetes
  `500m` is `0.5`), plus overridable `SyStats.CgroupRootPath` and
  `SyStats.SelfCgroupPath`. Falls back silently to host-wide values when
  there's no cgroup or no limit configured; check `Limited` to tell
  which you got.
- `CPU.Load1`/`Load5`/`Load15` - the traditional Unix load average from
  `/proc/loadavg` (what `uptime` shows), alongside the existing
  `LoadAvg`/`CoreAvg` which are CPU *utilization* percentages. Adds
  overridable `SyStats.LoadAvgPath`. These stay host-wide even with
  `ContainerAware` set - there's no cgroup equivalent.
- `SyStats.ProcessCPUMode` - selects how `GetTopProcesses` computes CPU
  usage: `CPUUsageInstant` (default, a live sampling window, like `top`)
  or `CPUUsageAverage` (lifetime average since process start, like
  `ps`'s default `%cpu` - much faster, single `/proc` read, no sampling
  wait).
- `SyStats.MountsPath` - overridable path to the mounts file used by
  `GetDisks` (defaults to `/proc/mounts`).

### Changed
- **`CPU.NoOfCores` now means logical CPUs** (what `nproc` reports), not
  physical cores. It previously came from `/proc/cpuinfo`'s `cpu cores`
  field, which is physical cores *per socket* - so it disagreed with
  `len(CoreAvg)` on any hyperthreaded machine and was half the real
  count on a dual-socket one. Use the new `CPU.PhysicalCores` if you
  want the old-style physical count; it's now computed correctly across
  all sockets.
- **JSON output now uses explicit lowerCamelCase tags** (e.g. `"rxBytes"`)
  instead of Go's default PascalCase field names (e.g. `"RxBytes"`) on
  every exported struct. This changes the JSON shape for any consumer
  deserializing these types - update accordingly.
- `go.mod` now requires Go 1.18 (previously 1.16), picked up by the
  `golang.org/x/sys` dependency added for the `df` replacement.
- `exec.ExecuteWithPipeAndError` dropped its `params` argument - it was
  never wired to anything and no caller used it.

### Fixed
- `canConnect`/`CanConnectExternal` no longer panics with a nil-pointer
  dereference when the HTTP request fails; requests now also time out
  after 10s instead of blocking forever.
- `fileops.IsFile` no longer leaks a file descriptor on every call, and
  now correctly excludes directories.
- Parse failures inside `strops.ToUint64`/`ToFloat64` (malformed `/proc`
  content) are now recovered as a normal returned error instead of
  crashing the calling process.
- `network.go`'s `readAsString` no longer returns the literal string
  `"error"` as if it were real data on a failed read - it now returns
  `""`, consistent with how numeric reads already zero out.
- CI's pinned Go version (1.17) no longer mismatches `go.mod`'s
  requirement (1.18) - CI was very likely failing on every push before
  this fix. CI now also runs `go vet` and a `gofmt` check.
- Subprocess output (`systemctl`, `service`, `who`, etc.) is now parsed
  under a forced `LC_ALL=C` locale, so a non-English host locale can no
  longer silently break string matching (e.g. `service.go`'s
  `"Active: active"` check).
- All subprocess calls now have a default 5s timeout instead of being
  able to hang indefinitely.

## [v0.2.0] - 2022-07-11

For `v0.2.0` and earlier, see the [git tags](../../tags) or
[releases](../../releases).
