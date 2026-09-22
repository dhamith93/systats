package systats

import (
	"testing"

	"github.com/dhamith93/systats/internal/fileops"
)

func TestParseCPUTopology(t *testing.T) {
	cases := []struct {
		name          string
		fixture       string
		logical       int
		physicalCores int
		sockets       int
	}{
		// The regression anchor: 8 processor entries, siblings 8, but
		// "cpu cores: 4". The old code reported 4 here while CoreAvg had
		// 8 entries - the bug this whole change exists to fix.
		{"single socket with HT", "./test_files/cpuinfo.txt", 8, 4, 1},
		// 2 sockets x 4 cores x 2 threads. "cpu cores: 4" is per-socket,
		// so the old code reported 4 - a quarter of the logical count and
		// half the physical count.
		{"dual socket", "./test_files/cpuinfo_dual_socket.txt", 16, 8, 2},
		// ARM publishes no physical id / core id at all, so topology
		// falls back to one core per logical CPU on a single socket
		// rather than reporting zero.
		{"arm without topology fields", "./test_files/cpuinfo_arm.txt", 4, 4, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseCPUTopology(fileops.ReadFile(c.fixture))
			if got.logical != c.logical {
				t.Errorf("logical = %d, want %d", got.logical, c.logical)
			}
			if got.physicalCores != c.physicalCores {
				t.Errorf("physicalCores = %d, want %d", got.physicalCores, c.physicalCores)
			}
			if got.sockets != c.sockets {
				t.Errorf("sockets = %d, want %d", got.sockets, c.sockets)
			}
		})
	}
}

func TestParseCPUTopologyEmptyInput(t *testing.T) {
	got := parseCPUTopology("")
	if got.logical != 0 || got.physicalCores != 0 || got.sockets != 0 {
		t.Errorf("got %+v, want all zeros so the caller's fallback kicks in", got)
	}
}

// TestParseCPUTopologyNoTrailingNewline guards the flush() at EOF - a
// single block with no trailing blank line must still be counted.
func TestParseCPUTopologyNoTrailingNewline(t *testing.T) {
	got := parseCPUTopology("processor\t: 0\nphysical id\t: 0\ncore id\t\t: 0")
	if got.logical != 1 || got.physicalCores != 1 || got.sockets != 1 {
		t.Errorf("got %+v, want {1 1 1}", got)
	}
}

// TestProcessCPUInfoFileContentFallback covers architectures whose
// /proc/cpuinfo has no "processor" lines at all (s390x, some masked
// containers) - the count then comes from /proc/stat's per-CPU lines.
func TestProcessCPUInfoFileContentFallback(t *testing.T) {
	content := "vendor_id\t: IBM/S390\nbogomips per cpu: 3331.00\n"
	output := CPU{}
	processCPUInfoFileContent(&output, &content, 12)

	if output.NoOfCores != 12 {
		t.Errorf("NoOfCores = %d, want 12 (from the fallback)", output.NoOfCores)
	}
	if output.PhysicalCores != 12 {
		t.Errorf("PhysicalCores = %d, want 12", output.PhysicalCores)
	}
	if output.Sockets != 1 {
		t.Errorf("Sockets = %d, want 1", output.Sockets)
	}
}

// TestProcessCPUInfoFileContentKeepsDescriptiveFields makes sure adding
// topology parsing didn't disturb the model/freq/cache extraction.
func TestProcessCPUInfoFileContentKeepsDescriptiveFields(t *testing.T) {
	output := CPU{}
	content := fileops.ReadFile("./test_files/cpuinfo.txt")
	processCPUInfoFileContent(&output, &content, 0)

	if output.Model != "Intel(R) Core(TM) i3-10100F CPU @ 3.60GHz" {
		t.Errorf("Model = %q", output.Model)
	}
	if output.NoOfCores != 8 {
		t.Errorf("NoOfCores = %d, want 8 (logical, not the 4 from \"cpu cores\")", output.NoOfCores)
	}
	if output.PhysicalCores != 4 {
		t.Errorf("PhysicalCores = %d, want 4", output.PhysicalCores)
	}
}
