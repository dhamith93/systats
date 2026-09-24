.PHONY: build-test run-test run-test-linux docker-test docker-test-host example example-serve example-linux

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

# Run the harness in a container that can see the host's other containers:
# the host's pid namespace (/proc for every container), its cgroup tree,
# and the Docker socket for names. SYS_PTRACE lets mount usage be read
# through /proc/<pid>/root.
docker-test-host:
	docker run --rm -v $(CURDIR):/src -w /src \
		--pid=host --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:ro \
		-v /var/run/docker.sock:/var/run/docker.sock --cap-add SYS_PTRACE \
		golang:1.24 go run ./build_test

# Render the example dashboard to example/dashboard.html
example:
	go run ./example

# Serve the example dashboard, re-collecting on every request
example-serve:
	go run ./example -serve :8080

# Cross-compile the example for a Linux host; the template is embedded,
# so the resulting binary is the only file you need to copy over
example-linux:
	GOOS=linux GOARCH=amd64 go build -o example/systats-dashboard-linux ./example
