package systats

import (
	"testing"

	"github.com/dhamith93/systats/internal/fileops"
)

func TestParseMounts(t *testing.T) {
	content := fileops.ReadFile("./test_files/mounts.txt")
	entries := parseMounts(content)

	want := []mountEntry{
		{device: "sysfs", mountPoint: "/sys", fsType: "sysfs"},
		{device: "proc", mountPoint: "/proc", fsType: "proc"},
		{device: "udev", mountPoint: "/dev", fsType: "devtmpfs"},
		{device: "tmpfs", mountPoint: "/run", fsType: "tmpfs"},
		{device: "/dev/sda2", mountPoint: "/", fsType: "ext4"},
		{device: "/dev/sda1", mountPoint: "/boot/efi", fsType: "vfat"},
		{device: "/dev/sdb1", mountPoint: "/mnt/My Backup Drive", fsType: "ext4"},
	}

	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i, e := range entries {
		if e != want[i] {
			t.Errorf("entry %d: got %+v, want %+v", i, e, want[i])
		}
	}
}

func TestUsagePercent(t *testing.T) {
	cases := []struct {
		used, avail uint64
		want        string
	}{
		{used: 50, avail: 50, want: "50%"},
		{used: 1, avail: 99, want: "1%"},
		{used: 1, avail: 999, want: "1%"},
		{used: 999, avail: 1, want: "100%"},
		{used: 1, avail: 2, want: "34%"}, // ceil(100/3) = 34, not truncated 33
		{used: 0, avail: 0, want: "0%"},
	}

	for _, c := range cases {
		got := usagePercent(c.used, c.avail)
		if got != c.want {
			t.Errorf("usagePercent(%d, %d) = %q, want %q", c.used, c.avail, got, c.want)
		}
	}
}

func TestExcludedFsTypesMatchesOldDfExcludeList(t *testing.T) {
	for _, fsType := range []string{"tmpfs", "devtmpfs", "udev"} {
		if !excludedFsTypes[fsType] {
			t.Errorf("expected %q to be excluded (matches old df --exclude-type list)", fsType)
		}
	}
	if excludedFsTypes["ext4"] {
		t.Errorf("ext4 should not be excluded")
	}
}
