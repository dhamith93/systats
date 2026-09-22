package unitconv

import "testing"

// Every expectation here is exact rather than approximate: these are all
// divisions by powers of two, which float64 represents without loss.

func TestKibToBytes(t *testing.T) {
	cases := map[uint64]float64{
		0:    0,
		1:    1024,
		1024: 1048576,
	}
	for input, want := range cases {
		if got := KibToBytes(input); got != want {
			t.Errorf("KibToBytes(%d) = %v, want %v", input, got, want)
		}
	}
}

// TestKibToKBIsIdentity pins the fact that makes the rest of this package
// make sense: /proc/meminfo labels its values "kB" but they are already
// KiB, so there is nothing to convert.
func TestKibToKBIsIdentity(t *testing.T) {
	for _, input := range []uint64{0, 1, 16315340} {
		if got := KibToKB(input); got != float64(input) {
			t.Errorf("KibToKB(%d) = %v, want %d", input, got, input)
		}
	}
}

func TestKibToMB(t *testing.T) {
	cases := []struct {
		name  string
		input uint64
		want  float64
	}{
		// The regression case: 512 MiB is 524288 KiB. The old
		// implementation multiplied by 1.024 and then divided by a binary
		// 1024, reporting 524 - neither 512 MiB nor 536.87 MB.
		{"512 MiB", 524288, 512},
		{"1 GiB", 1048576, 1024},
		{"1 MiB", 1024, 1},
		// Previously truncated to 0 by integer division.
		{"fraction of a MiB is kept", 1023, 0.9990234375},
		{"real meminfo MemTotal", 16315340, 15932.94921875},
		{"zero", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := KibToMB(c.input); got != c.want {
				t.Errorf("KibToMB(%d) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

func TestKibToGB(t *testing.T) {
	cases := []struct {
		name  string
		input uint64
		want  float64
	}{
		{"1 GiB", 1048576, 1},
		{"8 GiB", 8388608, 8},
		// The case that prompted the float64 change: this used to report
		// 3, discarding 21% of the value.
		{"3.82 GiB", 4010212, 3.8244361877441406},
		{"fraction of a GiB is kept", 524288, 0.5},
		{"zero", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := KibToGB(c.input); got != c.want {
				t.Errorf("KibToGB(%d) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

// TestConversionsAgree checks the units stay consistent with each other,
// which is the property the old mixed decimal/binary math broke.
func TestConversionsAgree(t *testing.T) {
	// Deliberately not a round number, so truncation would show up.
	const kib = 4010212

	if KibToBytes(kib) != KibToKB(kib)*1024 {
		t.Errorf("bytes and KB disagree: %v vs %v", KibToBytes(kib), KibToKB(kib)*1024)
	}
	if KibToKB(kib) != KibToMB(kib)*1024 {
		t.Errorf("KB and MB disagree: %v vs %v", KibToKB(kib), KibToMB(kib)*1024)
	}
	if KibToMB(kib) != KibToGB(kib)*1024 {
		t.Errorf("MB and GB disagree: %v vs %v", KibToMB(kib), KibToGB(kib)*1024)
	}
}
