package systats

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	fixtureID1 = "1111111111111111111111111111111111111111111111111111111111111111" // docker, systemd driver, all limits
	fixtureID2 = "2222222222222222222222222222222222222222222222222222222222222222" // docker, cgroupfs driver, unlimited, frozen
	fixtureID3 = "3333333333333333333333333333333333333333333333333333333333333333" // stopped
	fixtureID4 = "4444444444444444444444444444444444444444444444444444444444444444" // podman conmon, not a container
	fixtureID5 = "5555555555555555555555555555555555555555555555555555555555555555" // podman, pid in nested cgroup
	fixtureID6 = "6666666666666666666666666666666666666666666666666666666666666666" // kubernetes + containerd, host network
	fixtureID7 = "7777777777777777777777777777777777777777777777777777777777777777" // cgroup v1 docker
)

// containerFixture points every path GetContainers touches at the
// test_files/containers tree. The CPU window is tiny because the fixture
// counters are static - the sample is exercised by
// TestApplyContainerCPUSample instead.
func containerFixture(cgroupDir string) *SyStats {
	s := New()
	s.CgroupRootPath = "./test_files/containers/" + cgroupDir
	s.SelfCgroupPath = "./test_files/containers/" + cgroupDir + "/self_cgroup.txt"
	s.ProcPath = "./test_files/containers/proc"
	s.StatFilePath = "./test_files/stat.txt"
	s.MeminfoPath = "./test_files/meminfo.txt"
	s.DiskStatsPath = "./test_files/diskstats_14field.txt"
	s.ContainerSocketPath = ""
	s.CPUSampleWindow = time.Millisecond
	return &s
}

func byID(t *testing.T, containers []Container) map[string]Container {
	t.Helper()
	m := map[string]Container{}
	for _, c := range containers {
		m[c.ID] = c
	}
	return m
}

func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestMatchContainerCgroup(t *testing.T) {
	cases := []struct {
		relPath     string
		wantID      string
		wantRuntime string
	}{
		{"/system.slice/docker-" + fixtureID1 + ".scope", fixtureID1, "docker"},
		{"/docker/" + fixtureID2, fixtureID2, "docker"},
		{"/machine.slice/libpod-" + fixtureID5 + ".scope", fixtureID5, "podman"},
		{"/libpod_parent/libpod-" + fixtureID5, fixtureID5, "podman"},
		{"/kubepods.slice/kubepods-pod0a1b2c3d_4e5f_6789_abcd_ef0123456789.slice/cri-containerd-" + fixtureID6 + ".scope", fixtureID6, "containerd"},
		{"/kubepods.slice/kubepods-besteffort.slice/x/crio-" + fixtureID6 + ".scope", fixtureID6, "cri-o"},
		{"/kubepods/burstable/pod0a1b2c3d-4e5f-6789-abcd-ef0123456789/" + fixtureID6, fixtureID6, "kubernetes"},
		{"/lxc.payload.web01", "web01", "lxc"},
		{`/machine.slice/machine-debian\x2dtest.scope`, "debian-test", "machined"},
	}
	for _, c := range cases {
		id, runtime, ok := matchContainerCgroup(c.relPath)
		if !ok || id != c.wantID || runtime != c.wantRuntime {
			t.Errorf("matchContainerCgroup(%q) = (%q, %q, %v), want (%q, %q, true)", c.relPath, id, runtime, ok, c.wantID, c.wantRuntime)
		}
	}

	for _, relPath := range []string{
		"/machine.slice/libpod-conmon-" + fixtureID4 + ".scope",
		"/system.slice/docker.service",
		"/system.slice/docker-" + fixtureID1[:12] + ".scope", // short IDs are never cgroup names
		"/user.slice/" + fixtureID1,                          // bare ID outside docker/ or kubepods
		"/system.slice/machine-foo.scope",                    // machine- outside machine.slice
		"/kubepods.slice/kubepods-burstable.slice",
	} {
		if id, runtime, ok := matchContainerCgroup(relPath); ok {
			t.Errorf("matchContainerCgroup(%q) matched (%q, %q), want no match", relPath, id, runtime)
		}
	}
}

func TestPodUIDFromCgroupPath(t *testing.T) {
	want := "0a1b2c3d-4e5f-6789-abcd-ef0123456789"
	for _, p := range []string{
		"/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod0a1b2c3d_4e5f_6789_abcd_ef0123456789.slice/cri-containerd-x.scope",
		"/kubepods/burstable/pod0a1b2c3d-4e5f-6789-abcd-ef0123456789/abc",
	} {
		if got := podUIDFromCgroupPath(p); got != want {
			t.Errorf("podUIDFromCgroupPath(%q) = %q, want %q", p, got, want)
		}
	}
	if got := podUIDFromCgroupPath("/system.slice/docker-x.scope"); got != "" {
		t.Errorf("expected no pod UID outside kubepods, got %q", got)
	}
}

func TestDiscoverContainersV2(t *testing.T) {
	refs := discoverContainers(containerFixture("cgroup_v2"), cgroupV2)

	got := map[string]containerRef{}
	for _, r := range refs {
		got[r.id] = r
	}
	if len(got) != 4 {
		t.Fatalf("discovered %d containers, want 4 (1, 2, 5, 6): %+v", len(got), refs)
	}
	for _, id := range []string{fixtureID3, fixtureID4} {
		if _, ok := got[id]; ok {
			t.Errorf("container %s... should not be discovered", id[:4])
		}
	}

	if r := got[fixtureID5]; r.pid != 5252 || r.runtime != "podman" {
		t.Errorf("podman: got pid=%d runtime=%q, want the nested cgroup's pid 5252 and podman", r.pid, r.runtime)
	}
	if r := got[fixtureID1]; r.pid != 4242 || r.dirs.memory != filepath.Join("test_files/containers/cgroup_v2/system.slice", "docker-"+fixtureID1+".scope") {
		t.Errorf("docker: got pid=%d memory dir=%q", r.pid, r.dirs.memory)
	}
	if r := got[fixtureID6]; r.podUID != "0a1b2c3d-4e5f-6789-abcd-ef0123456789" || r.runtime != "containerd" {
		t.Errorf("kubernetes: got podUID=%q runtime=%q", r.podUID, r.runtime)
	}
}

func TestGetContainersV2(t *testing.T) {
	s := containerFixture("cgroup_v2")
	containers, err := s.GetContainers(Megabyte)
	if err != nil {
		t.Fatalf("GetContainers returned %v", err)
	}
	all := byID(t, containers)
	if len(all) != 4 {
		t.Fatalf("got %d containers, want 4", len(all))
	}

	c := all[fixtureID1]
	if c.ShortID != "111111111111" || c.Runtime != "docker" || c.State != "running" || c.MetadataAvailable || c.Pid != 4242 {
		t.Errorf("identity: got %+v", c)
	}

	m := c.Memory
	if !m.Limited || !approx(m.Limit, 256) || !approx(m.Used, 90) || m.Unit != Megabyte {
		t.Errorf("memory: got limited=%v limit=%v used=%v unit=%v, want true 256 90 MB", m.Limited, m.Limit, m.Used, m.Unit)
	}
	if !approx(m.PercentageUsed, 90.0/256*100) {
		t.Errorf("memory: PercentageUsed = %v, want %v", m.PercentageUsed, 90.0/256*100)
	}
	if !approx(m.Anon, 80) || !approx(m.File, 20) || !approx(m.Swap, 1) || m.OOMKills != 2 {
		t.Errorf("memory breakdown: got anon=%v file=%v swap=%v oom=%d, want 80 20 1 2", m.Anon, m.File, m.Swap, m.OOMKills)
	}

	cpu := c.CPU
	if !cpu.Limited || cpu.AllocatedCores != 0.5 {
		t.Errorf("cpu: got limited=%v cores=%v, want true 0.5", cpu.Limited, cpu.AllocatedCores)
	}
	if cpu.UsageSeconds != 90 || cpu.UserSeconds != 60 || cpu.SystemSeconds != 30 {
		t.Errorf("cpu: got usage=%v user=%v system=%v, want 90 60 30", cpu.UsageSeconds, cpu.UserSeconds, cpu.SystemSeconds)
	}
	if cpu.Periods != 1000 || cpu.ThrottledPeriods != 250 || cpu.ThrottledSeconds != 4.5 {
		t.Errorf("cpu throttling: got %d/%d %vs, want 1000/250 4.5s", cpu.Periods, cpu.ThrottledPeriods, cpu.ThrottledSeconds)
	}
	if cpu.CoresUsed != 0 {
		t.Errorf("cpu: static counters should give CoresUsed 0, got %v", cpu.CoresUsed)
	}

	if c.Pids != (ContainerPids{Current: 17, Max: 100, Limited: true}) {
		t.Errorf("pids: got %+v", c.Pids)
	}

	if len(c.BlockIO) != 2 {
		t.Fatalf("blockIO: got %+v, want 2 devices", c.BlockIO)
	}
	if io := c.BlockIO[0]; io.Device != "sda" || io.ReadBytes != 1048576 || io.WriteBytes != 2097152 || io.Reads != 10 || io.Writes != 20 {
		t.Errorf("blockIO[0]: got %+v", io)
	}
	if io := c.BlockIO[1]; io.Device != "253:7" {
		t.Errorf("blockIO[1]: a device missing from diskstats should be named by number, got %q", io.Device)
	}

	if !c.Pressure.Available || !c.Pressure.Limited || c.Pressure.CPU.Some.Avg10 != 12 || c.Pressure.Memory.Full.Total != 100 {
		t.Errorf("pressure: got %+v", c.Pressure)
	}

	n := c.Network
	if !n.Accessible || n.SharesHostNetwork || len(n.Interfaces) != 1 {
		t.Fatalf("network: got %+v, want one accessible non-loopback interface in its own namespace", n)
	}
	if i := n.Interfaces[0]; i.Interface != "eth0" || i.RxBytes != 1500000 || i.TxBytes != 300000 ||
		i.RxPackets != 1200 || i.TxPackets != 900 || i.RxErrors != 1 || i.RxDropped != 2 || i.TxErrors != 3 || i.TxDropped != 4 {
		t.Errorf("network eth0: got %+v", i)
	}

	mounts := map[string]ContainerMount{}
	for _, mt := range c.Mounts {
		mounts[mt.MountPoint] = mt
	}
	if len(mounts) != 3 {
		t.Fatalf("mounts: got %+v, want /, /data and /missing volume only", c.Mounts)
	}
	for _, mp := range []string{"/", "/data"} {
		if mt := mounts[mp]; !mt.Accessible || mt.Total <= 0 || mt.Unit != Megabyte {
			t.Errorf("mount %s: got %+v, want accessible with a size", mp, mt)
		}
	}
	if mt := mounts["/missing volume"]; mt.Accessible || mt.Total != 0 || mt.FSType != "xfs" || mt.Device != "/dev/sdb1" {
		t.Errorf("unstattable mount: got %+v, want inaccessible, zero size", mt)
	}

	// cgroupfs docker: no limits, frozen, and no /proc entry for its pid.
	c = all[fixtureID2]
	if c.State != "paused" {
		t.Errorf("frozen container: State = %q, want paused", c.State)
	}
	hostMB := 16315340.0 / 1024
	if c.Memory.Limited || !approx(c.Memory.Limit, hostMB) || !approx(c.Memory.Used, 50) {
		t.Errorf("unlimited memory: got %+v, want Limit = host total %v", c.Memory, hostMB)
	}
	if c.CPU.Limited || c.Pids.Limited || c.Pids.Current != 3 {
		t.Errorf("unlimited cpu/pids: got cpu=%+v pids=%+v", c.CPU, c.Pids)
	}
	if c.Network.Accessible || len(c.Mounts) != 0 {
		t.Errorf("missing /proc entry: got network=%+v mounts=%+v, want inaccessible and none", c.Network, c.Mounts)
	}

	c = all[fixtureID6]
	if !c.Network.SharesHostNetwork || c.CPU.AllocatedCores != 2 || c.PodUID == "" {
		t.Errorf("kubernetes: got sharesHost=%v cores=%v podUID=%q", c.Network.SharesHostNetwork, c.CPU.AllocatedCores, c.PodUID)
	}
}

func TestGetContainersV1(t *testing.T) {
	containers, err := containerFixture("cgroup_v1").GetContainers(Megabyte)
	if err != nil {
		t.Fatalf("GetContainers returned %v", err)
	}
	if len(containers) != 1 || containers[0].ID != fixtureID7 {
		t.Fatalf("got %+v, want only container 7", containers)
	}
	c := containers[0]

	m := c.Memory
	if !m.Limited || !approx(m.Limit, 256) || !approx(m.Used, 90) || !approx(m.Anon, 80) || !approx(m.File, 20) || !approx(m.Swap, 2) || m.OOMKills != 1 {
		t.Errorf("memory: got %+v", m)
	}

	cpu := c.CPU
	if !cpu.Limited || cpu.AllocatedCores != 0.5 || cpu.UsageSeconds != 90 || cpu.UserSeconds != 60 || cpu.SystemSeconds != 30 {
		t.Errorf("cpu: got %+v", cpu)
	}
	if cpu.Periods != 1000 || cpu.ThrottledPeriods != 250 || cpu.ThrottledSeconds != 4.5 {
		t.Errorf("cpu throttling: got %+v", cpu)
	}

	if c.Pids != (ContainerPids{Current: 17, Max: 100, Limited: true}) {
		t.Errorf("pids: got %+v", c.Pids)
	}
	want := ContainerBlockIO{Device: "sda", Major: 8, Minor: 0, ReadBytes: 1048576, WriteBytes: 2097152, Reads: 10, Writes: 20}
	if len(c.BlockIO) != 1 || c.BlockIO[0] != want {
		t.Errorf("blockIO: got %+v, want [%+v]", c.BlockIO, want)
	}
	if c.Pressure.Available {
		t.Errorf("cgroup v1 has no PSI, got Available=true")
	}
	if c.State != "running" || !c.Network.Accessible {
		t.Errorf("got state=%q network=%+v", c.State, c.Network)
	}
}

func TestGetContainersErrors(t *testing.T) {
	s := containerFixture("cgroup_v2")
	if _, err := s.GetContainers(Unit("PB")); err == nil {
		t.Errorf("expected an error for an unsupported unit")
	}

	none := containerFixture("does_not_exist")
	if _, err := none.GetContainers(Megabyte); err == nil {
		t.Errorf("expected an error with no cgroup filesystem")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	slow := containerFixture("cgroup_v2")
	slow.CPUSampleWindow = time.Hour
	if _, err := slow.GetContainersWithContext(ctx, Megabyte); err == nil {
		t.Errorf("expected a cancelled context to abort the sampling window")
	}
}

func TestGetContainersEmptyHost(t *testing.T) {
	// The ContainerAware fixture is a valid cgroup tree with no containers.
	s := containerFixture("")
	s.CgroupRootPath = "./test_files/cgroup_v2"
	containers, err := s.GetContainers(Megabyte)
	if err != nil || containers == nil || len(containers) != 0 {
		t.Errorf("got (%v, %v), want an empty non-nil slice and no error", containers, err)
	}
}

func TestMatchContainer(t *testing.T) {
	containers := []Container{
		{ID: "abc111" + strings.Repeat("0", 58), Name: "web"},
		{ID: "abc222" + strings.Repeat("0", 58), Name: "db"},
		{ID: "def333" + strings.Repeat("0", 58)},
	}
	cases := []struct {
		query   string
		want    int
		wantErr bool
	}{
		{containers[1].ID, 1, false},
		{"web", 0, false},
		{"db", 1, false},
		{"abc2", 1, false},
		{"def", 2, false},
		{"abc", 0, true}, // ambiguous
		{"zzz", 0, true},
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := matchContainer(containers, c.query)
		if (err != nil) != c.wantErr || (!c.wantErr && got != c.want) {
			t.Errorf("matchContainer(%q) = (%d, %v), want (%d, err=%v)", c.query, got, err, c.want, c.wantErr)
		}
	}
}

func TestGetContainer(t *testing.T) {
	s := containerFixture("cgroup_v2")
	c, err := s.GetContainer(Megabyte, "666666")
	if err != nil || c.ID != fixtureID6 {
		t.Errorf("GetContainer by prefix: got (%s, %v)", c.ID, err)
	}
	if _, err := s.GetContainer(Megabyte, fixtureID3); err == nil {
		t.Errorf("expected an error for a stopped container")
	}
}

// serveContainerAPI serves body as /containers/json on a unix socket and
// returns the socket path.
func serveContainerAPI(t *testing.T, status int, body string) string {
	t.Helper()
	// os.MkdirTemp rather than t.TempDir: macOS caps unix socket paths at
	// 104 bytes, and t.TempDir's paths include the full test name.
	dir, err := os.MkdirTemp("", "systats")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "api.sock")

	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	srv.Listener.Close()
	srv.Listener = listener
	srv.Start()
	t.Cleanup(srv.Close)
	return sock
}

func TestGetContainersMetadata(t *testing.T) {
	body, _ := json.Marshal([]apiContainer{{
		ID: fixtureID1, Names: []string{"/web"}, Image: "nginx:1.27", State: "running",
		Labels: map[string]string{"com.docker.compose.service": "web"},
	}})
	s := containerFixture("cgroup_v2")
	s.ContainerSocketPath = serveContainerAPI(t, http.StatusOK, string(body))

	containers, err := s.GetContainers(Megabyte)
	if err != nil {
		t.Fatalf("GetContainers returned %v", err)
	}
	all := byID(t, containers)
	web := all[fixtureID1]
	if !web.MetadataAvailable || web.Name != "web" || web.Image != "nginx:1.27" || web.Labels["com.docker.compose.service"] != "web" {
		t.Errorf("metadata not joined: got %+v", web)
	}
	// Known to the cgroup tree but not to the API (another runtime).
	if other := all[fixtureID6]; other.MetadataAvailable || other.Name != "" {
		t.Errorf("container unknown to the API should have no metadata, got %+v", other)
	}

	byName, err := s.GetContainer(Megabyte, "web")
	if err != nil || byName.ID != fixtureID1 {
		t.Errorf("GetContainer by name: got (%s, %v)", byName.ID, err)
	}
}

func TestGetContainersMetadataFailuresAreNotErrors(t *testing.T) {
	cases := map[string]string{
		"missing socket": filepath.Join(os.TempDir(), "systats-no-such.sock"),
		"server error":   serveContainerAPI(t, http.StatusInternalServerError, `{"message":"boom"}`),
		"garbage body":   serveContainerAPI(t, http.StatusOK, `not json`),
	}
	for name, sock := range cases {
		t.Run(name, func(t *testing.T) {
			s := containerFixture("cgroup_v2")
			s.ContainerSocketPath = sock
			containers, err := s.GetContainers(Megabyte)
			if err != nil || len(containers) != 4 {
				t.Fatalf("got (%d containers, %v), want 4 and no error", len(containers), err)
			}
			for _, c := range containers {
				if c.MetadataAvailable {
					t.Errorf("%s: MetadataAvailable = true", c.ShortID)
				}
			}
		})
	}
}

func TestApplyContainerCPUSample(t *testing.T) {
	out := ContainerCPU{Limited: true, AllocatedCores: 0.5}
	// 0.3 cores used over 1s: 60% of a 0.5-core quota, 3.75% of 8 host cores.
	applyContainerCPUSample(&out, 10.0, 10.3, true, 1.0, 8)
	if !approx(out.CoresUsed, 0.3) || !approx(out.PercentOfLimit, 60) || !approx(out.PercentOfHost, 3.75) {
		t.Errorf("got %+v, want 0.3 cores, 60%% of limit, 3.75%% of host", out)
	}
	if out.UsageSeconds != 10.3 {
		t.Errorf("UsageSeconds = %v, want the latest reading 10.3", out.UsageSeconds)
	}

	unlimited := ContainerCPU{}
	applyContainerCPUSample(&unlimited, 0, 2, true, 1.0, 8)
	if unlimited.PercentOfLimit != 0 || !approx(unlimited.PercentOfHost, 25) {
		t.Errorf("unlimited: got %+v, want 0%% of limit, 25%% of host", unlimited)
	}

	for name, args := range map[string]struct {
		u1, u2 float64
		ok     bool
		secs   float64
	}{
		"backwards":   {5, 4, true, 1},
		"failed read": {1, 2, false, 1},
		"no time":     {1, 2, true, 0},
	} {
		got := ContainerCPU{Limited: true, AllocatedCores: 1}
		applyContainerCPUSample(&got, args.u1, args.u2, args.ok, args.secs, 8)
		if got.CoresUsed != 0 || got.PercentOfHost != 0 || got.PercentOfLimit != 0 {
			t.Errorf("%s: got %+v, want zero usage", name, got)
		}
	}
}

func TestContainerRatesSince(t *testing.T) {
	prev := Container{
		ID:      "a",
		Network: ContainerNetwork{Interfaces: []ContainerInterface{{RxBytes: 1000, TxBytes: 500, RxPackets: 10, TxPackets: 5}, {RxBytes: 1000}}},
		BlockIO: []ContainerBlockIO{{ReadBytes: 4096, WriteBytes: 0, Reads: 1, Writes: 0}},
		CPU:     ContainerCPU{ThrottledPeriods: 10, ThrottledSeconds: 1},
	}
	now := Container{
		ID:      "a",
		Network: ContainerNetwork{Interfaces: []ContainerInterface{{RxBytes: 3000, TxBytes: 1500, RxPackets: 30, TxPackets: 15}, {RxBytes: 3000}}},
		BlockIO: []ContainerBlockIO{{ReadBytes: 8192, WriteBytes: 8192, Reads: 3, Writes: 4}},
		CPU:     ContainerCPU{ThrottledPeriods: 30, ThrottledSeconds: 2},
	}

	r := now.RatesSince(prev, 2)
	want := ContainerRates{
		ID: "a", RxBytesPerSec: 2000, TxBytesPerSec: 500, RxPacketsPerSec: 10, TxPacketsPerSec: 5,
		ReadBytesPerSec: 2048, WriteBytesPerSec: 4096, ReadsPerSec: 1, WritesPerSec: 2,
		CPUThrottledPerSec: 10, CPUThrottledSecPerSec: 0.5,
	}
	if r != want {
		t.Errorf("got %+v, want %+v", r, want)
	}

	// A recreated interface resets the network counters, but block I/O is
	// still valid and must not be thrown away with it.
	reset := now
	reset.Network = ContainerNetwork{Interfaces: []ContainerInterface{{RxBytes: 10}}}
	r = reset.RatesSince(prev, 2)
	if r.RxBytesPerSec != 0 || r.TxBytesPerSec != 0 || r.ReadBytesPerSec != 2048 {
		t.Errorf("network reset: got %+v, want zero network rates and intact block rates", r)
	}

	if r := now.RatesSince(Container{ID: "b"}, 2); r != (ContainerRates{ID: "a"}) {
		t.Errorf("different container: got %+v, want zeros", r)
	}
	if r := now.RatesSince(prev, 0); r != (ContainerRates{ID: "a"}) {
		t.Errorf("zero elapsed: got %+v, want zeros", r)
	}
}

func TestParseNetDevSkipsHeadersAndShortLines(t *testing.T) {
	content := "Inter-|   Receive  |  Transmit\n face |bytes packets|bytes\n  eth0: 1 2 3\n    lo: 1 1 0 0 0 0 0 0 1 1 0 0 0 0 0 0\n"
	if got := parseNetDev(content); len(got) != 0 {
		t.Errorf("got %+v, want no interfaces", got)
	}
}

func TestParseBlkioThrottleIgnoresTotals(t *testing.T) {
	got := parseBlkioThrottle("8:0 Read 5\n8:16 Write 7\nTotal 12\n", "8:0 Read 1\n8:16 Write 2\nTotal 3\n")
	want := []ContainerBlockIO{
		{Major: 8, Minor: 0, ReadBytes: 5, Reads: 1},
		{Major: 8, Minor: 16, WriteBytes: 7, Writes: 2},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestV1ControllerDirNames(t *testing.T) {
	names := v1ControllerDirNames("11:memory:/\n5:cpuacct,cpu:/\n1:name=systemd:/\n")
	if names["cpu"] != "cpuacct,cpu" || names["cpuacct"] != "cpuacct,cpu" || names["memory"] != "memory" {
		t.Errorf("got %v", names)
	}
	// Controllers absent from the file keep their conventional directory.
	if names["pids"] != "pids" || names["blkio"] != "blkio" {
		t.Errorf("fallbacks: got %v", names)
	}
}

func TestOverlayUpperDir(t *testing.T) {
	mounts := "overlay / overlay rw,lowerdir=/l,upperdir=/var/lib/docker/overlay2/My\\040Layer/diff,workdir=/w 0 0\n" +
		"overlay /data overlay rw,upperdir=/not/root 0 0\n"
	if got := overlayUpperDir(mounts); got != "/var/lib/docker/overlay2/My Layer/diff" {
		t.Errorf("got %q", got)
	}
	if got := overlayUpperDir("/dev/sda1 / ext4 rw 0 0\n"); got != "" {
		t.Errorf("non-overlay root: got %q, want \"\"", got)
	}
}

func TestGetContainersLayerSize(t *testing.T) {
	s := containerFixture("cgroup_v2")
	containers, err := s.GetContainers(Byte)
	if err != nil {
		t.Fatal(err)
	}
	if l := byID(t, containers)[fixtureID1].Layer; l.Available || l.Unit != Byte {
		t.Errorf("ContainerLayerSize off: got %+v, want not measured", l)
	}

	s.ContainerLayerSize = true
	containers, err = s.GetContainers(Byte)
	if err != nil {
		t.Fatal(err)
	}
	all := byID(t, containers)

	// Fixture layer: var/log/app.log (1000 bytes) + etc/app.conf (24 bytes).
	want := ContainerLayer{Size: 1024, Unit: Byte, Files: 2, Path: "/var/lib/docker/overlay2/x/diff", Available: true}
	if l := all[fixtureID1].Layer; l != want {
		t.Errorf("got %+v, want %+v", l, want)
	}
	// No mounts file for this pid, so no overlay root to measure.
	if l := all[fixtureID5].Layer; l.Available {
		t.Errorf("container without an overlay root: got %+v, want not measured", l)
	}
}

func TestLayerSizeCountsHardLinksOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "sub", "data")
	if err := os.WriteFile(file, make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(dir, "hardlink")); err != nil {
		t.Skipf("hard links unsupported: %v", err)
	}
	if err := os.Symlink(file, filepath.Join(dir, "symlink")); err != nil {
		t.Fatal(err)
	}

	size, files, err := layerSize(context.Background(), dir)
	if err != nil || size != 100 || files != 1 {
		t.Errorf("got (%d bytes, %d files, %v), want (100, 1, nil)", size, files, err)
	}

	if _, _, err := layerSize(context.Background(), filepath.Join(dir, "missing")); err == nil {
		t.Errorf("expected an error for an unreadable layer")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := layerSize(ctx, dir); err == nil {
		t.Errorf("expected a cancelled context to stop the walk")
	}
}
