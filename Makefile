.PHONY: build-test run-test run-test-linux docker-test

# Build the manual test harness for the current OS/arch (only a subset of
# calls will work on macOS - see build_test/main.go output for per-call errors)
build-test:
	go build -o build_test/systats-test ./build_test

# Build+run locally in one step
run-test:
	go run ./build_test

# Cross-compile a Linux binary you can scp/copy to a real Linux host or VM
run-test-linux:
	GOOS=linux GOARCH=amd64 go build -o build_test/systats-test-linux ./build_test

# Run the harness inside a disposable Linux container (needs docker/colima running)
docker-test:
	docker run --rm -v $(CURDIR):/src -w /src golang:1.24 go run ./build_test

docker-test-with-limits:
	docker run --rm -v $(CURDIR):/src -w /src --cpus=0.5 --memory=256m golang:1.24 go run ./build_test