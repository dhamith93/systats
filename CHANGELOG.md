# Changelog

All notable changes to this project are documented here, following the
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/) format.

## [Unreleased]

### Removed
- Shelled-out `ps`, `df`, `ip`, `lsof`, and `whereis`/`GetExecPath` -
  replaced with `/proc`, `/sys`, `net.Interfaces()`, and
  `golang.org/x/sys/unix.Statfs`. No external process is spawned for
  `GetCPU`, `GetMemory`, `GetSwap`, `GetDisks`, `GetNetworks`,
  `GetTopProcesses`, or `EstablishedTCPConnCount` anymore.

### Added
- `SyStats.ProcessCPUMode` - selects how `GetTopProcesses` computes CPU
  usage: `CPUUsageInstant` (default, a live sampling window, like `top`)
  or `CPUUsageAverage` (lifetime average since process start, like
  `ps`'s default `%cpu` - much faster, single `/proc` read, no sampling
  wait).
- `SyStats.MountsPath` - overridable path to the mounts file used by
  `GetDisks` (defaults to `/proc/mounts`).

### Changed
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

For `v0.2.0` and earlier, see the [git tags](../../tags) or
[releases](../../releases).

## [v0.2.0] - 2022-07-11
See git history.
