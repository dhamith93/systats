# Contributing

## Building and testing

```sh
go build ./...
go vet ./...
gofmt -l .    # should print nothing; any listed file needs `gofmt -w`
go test ./...
```

These are the same checks CI runs on every push/PR.

## A note on running tests locally

This library is Linux-only - most of it reads `/proc`, `/sys`, or shells
out to `systemctl`/`service`. If you're not on Linux, several tests will
fail with errors like `/proc/stat file not found` or `cannot find
systemctl` - that's expected, not a sign something is broken. Tests that
only depend on the fixture files in `test_files/` (memory, swap, system,
disk unit conversion) will pass anywhere.

For full local verification, run the suite on a real Linux host or VM.

## Pull requests

Keep changes focused and include tests where the change is testable
without a live Linux environment (parsing logic, pure calculations,
sorting, etc. can all be tested with synthetic data or fixture files -
see `process_test.go` or `disk_test.go` for examples).
