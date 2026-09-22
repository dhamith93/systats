package strops

import "testing"

func TestToUint64(t *testing.T) {
	cases := map[string]uint64{
		"0":                    0,
		"16315340":             16315340,
		"18446744073709551615": 1<<64 - 1, // max uint64, the /proc counter ceiling
	}
	for input, want := range cases {
		if got := ToUint64(input); got != want {
			t.Errorf("ToUint64(%q) = %d, want %d", input, got, want)
		}
	}
}

// Malformed /proc content panics rather than returning an error;
// systats.withRecover turns that into an error at the API boundary. These
// cases pin down what counts as malformed.
func TestToUint64PanicsOnBadInput(t *testing.T) {
	for _, input := range []string{"", "abc", "-1", "1.5", " 12", "12 ", "0x10"} {
		assertPanics(t, "ToUint64("+input+")", func() { ToUint64(input) })
	}
}

func TestToFloat64(t *testing.T) {
	cases := map[string]float64{
		"0":        0,
		"12058.79": 12058.79,
		"-1":       -1, // Tcp: MaxConn is -1 on most hosts
		"1.5e3":    1500,
	}
	for input, want := range cases {
		if got := ToFloat64(input); got != want {
			t.Errorf("ToFloat64(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestToFloat64PanicsOnBadInput(t *testing.T) {
	for _, input := range []string{"", "abc", "1.2.3", " 1.5"} {
		assertPanics(t, "ToFloat64("+input+")", func() { ToFloat64(input) })
	}
}

func assertPanics(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic, want a panic", name)
		}
	}()
	fn()
}
