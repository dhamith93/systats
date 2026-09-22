package main

import (
	"math"
	"strings"
	"testing"

	"github.com/dhamith93/systats"
)

func TestDashArray(t *testing.T) {
	const r = 54
	circumference := 2 * math.Pi * r

	cases := []struct {
		name    string
		pct     float64
		wantDsh float64
	}{
		{"empty", 0, 0},
		{"half", 50, circumference / 2},
		{"full", 100, circumference},
		{"quarter", 25, circumference / 4},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dash, gap := dashArray(c.pct, r)
			if math.Abs(dash-c.wantDsh) > 1e-9 {
				t.Errorf("dash = %v, want %v", dash, c.wantDsh)
			}
			// The two lengths must always add back up to the full circle,
			// or the arc leaves a visible seam.
			if math.Abs((dash+gap)-circumference) > 1e-9 {
				t.Errorf("dash+gap = %v, want the circumference %v", dash+gap, circumference)
			}
		})
	}
}

// Out-of-range input is reachable: CPU percentages can land slightly over
// 100, and a division by a zero-sized filesystem yields NaN. Either would
// wrap or blank the arc without clamping.
func TestDashArrayClampsOutOfRange(t *testing.T) {
	const r = 54
	circumference := 2 * math.Pi * r

	for _, pct := range []float64{101, 1000, math.NaN()} {
		dash, gap := dashArray(pct, r)
		if dash < 0 || dash > circumference+1e-9 {
			t.Errorf("dashArray(%v) dash = %v, want within [0, %v]", pct, dash, circumference)
		}
		if gap < 0 {
			t.Errorf("dashArray(%v) gap = %v, want >= 0", pct, gap)
		}
	}

	if dash, _ := dashArray(-5, r); dash != 0 {
		t.Errorf("dashArray(-5) dash = %v, want 0", dash)
	}
	if dash, _ := dashArray(math.NaN(), r); dash != 0 {
		t.Errorf("dashArray(NaN) dash = %v, want 0", dash)
	}
}

func TestLevelColor(t *testing.T) {
	cases := map[float64]string{
		0:    colorOK,
		69.9: colorOK,
		70:   colorWarn, // boundary is inclusive
		84.9: colorWarn,
		85:   colorCrit,
		100:  colorCrit,
		150:  colorCrit, // clamped, still critical
	}
	for pct, want := range cases {
		if got := levelColor(pct); got != want {
			t.Errorf("levelColor(%v) = %s, want %s", pct, got, want)
		}
	}
}

// A sensor that publishes no thresholds reports High and Critical as 0.
// Treating those as real limits would paint every reading critical, so
// the *Available flags have to drive the decision.
func TestTempColorWithoutThresholds(t *testing.T) {
	cool := systats.Temperature{Celsius: 43}
	if got := tempColor(cool); got != colorOK {
		t.Errorf("43C with no thresholds = %s, want %s", got, colorOK)
	}

	hot := systats.Temperature{Celsius: 92}
	if got := tempColor(hot); got != colorCrit {
		t.Errorf("92C with no thresholds = %s, want %s", got, colorCrit)
	}
}

func TestTempColorUsesPublishedThresholds(t *testing.T) {
	// Below both thresholds.
	ok := systats.Temperature{
		Celsius: 50,
		High:    80, HighAvailable: true,
		Critical: 100, CriticalAvailable: true,
	}
	if got := tempColor(ok); got != colorOK {
		t.Errorf("50C (high 80, crit 100) = %s, want %s", got, colorOK)
	}

	// Past high but not critical. Note 85 would be critical under the
	// fixed fallback scale - the chip's own numbers must win.
	warn := systats.Temperature{
		Celsius: 85,
		High:    80, HighAvailable: true,
		Critical: 100, CriticalAvailable: true,
	}
	if got := tempColor(warn); got != colorWarn {
		t.Errorf("85C (high 80, crit 100) = %s, want %s", got, colorWarn)
	}

	crit := systats.Temperature{
		Celsius: 100,
		High:    80, HighAvailable: true,
		Critical: 100, CriticalAvailable: true,
	}
	if got := tempColor(crit); got != colorCrit {
		t.Errorf("100C (crit 100) = %s, want %s", got, colorCrit)
	}
}

func TestTempScale(t *testing.T) {
	// Scaled against the critical threshold when there is one.
	pct, limits := tempScale(systats.Temperature{
		Celsius: 50, Critical: 100, CriticalAvailable: true,
	})
	if pct != 50 {
		t.Errorf("pct = %v, want 50", pct)
	}
	if !strings.Contains(limits, "crit 100") {
		t.Errorf("limits = %q, want it to mention the critical threshold", limits)
	}

	// No thresholds: 100C ceiling, and the caption says so rather than
	// implying a limit that was never published.
	pct, limits = tempScale(systats.Temperature{Celsius: 42})
	if pct != 42 {
		t.Errorf("pct = %v, want 42 against the 100C fallback ceiling", pct)
	}
	if !strings.Contains(limits, "no thresholds") {
		t.Errorf("limits = %q, want it to say no thresholds were published", limits)
	}

	// A chip reporting a zero critical must not divide by zero.
	pct, _ = tempScale(systats.Temperature{Celsius: 40, Critical: 0, CriticalAvailable: true})
	if math.IsInf(pct, 0) || math.IsNaN(pct) {
		t.Errorf("pct = %v, want a finite value", pct)
	}
}

func TestFmtFloat(t *testing.T) {
	cases := map[float64]string{
		0:       "0",
		1:       "1",
		1.5:     "1.5",
		1.25:    "1.25",
		339.292: "339.29",
		100:     "100",
	}
	for in, want := range cases {
		if got := fmtFloat(in); got != want {
			t.Errorf("fmtFloat(%v) = %q, want %q", in, got, want)
		}
	}

	// NaN and Inf reach here through the same divisions clampPercent
	// guards; emitting them into an SVG attribute would break the shape.
	for _, in := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := fmtFloat(in); got != "0" {
			t.Errorf("fmtFloat(%v) = %q, want %q", in, got, "0")
		}
	}
}

func TestBytesHuman(t *testing.T) {
	cases := map[float64]string{
		0:               "0 B",
		512:             "512 B",
		1024:            "1.00 KB",
		1536:            "1.50 KB",
		1024 * 1024:     "1.00 MB",
		4106457088:      "3.82 GB", // the VM from the original unit bug
		1024 * 1024 * 3: "3.00 MB",
	}
	for in, want := range cases {
		if got := bytesHuman(in); got != want {
			t.Errorf("bytesHuman(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestRate(t *testing.T) {
	if got := rate(0); got != "idle" {
		t.Errorf("rate(0) = %q, want %q", got, "idle")
	}
	if got := rate(-1); got != "idle" {
		t.Errorf("rate(-1) = %q, want %q", got, "idle")
	}
	if got := rate(1024 * 1024); got != "1.00 MB/s" {
		t.Errorf("rate(1MiB) = %q, want %q", got, "1.00 MB/s")
	}
}

func TestDeltaPerSec(t *testing.T) {
	if got := deltaPerSec(2048, 1024, 2); got != 512 {
		t.Errorf("deltaPerSec = %v, want 512", got)
	}
	// Counters reset when an interface is reconfigured. Subtracting
	// unsigned would underflow into an absurd rate.
	if got := deltaPerSec(10, 1000, 1); got != 0 {
		t.Errorf("deltaPerSec across a counter reset = %v, want 0", got)
	}
	if got := deltaPerSec(2048, 1024, 0); got != 0 {
		t.Errorf("deltaPerSec with zero elapsed = %v, want 0", got)
	}
}

func TestParseUnit(t *testing.T) {
	cases := map[string]systats.Unit{
		"B": systats.Byte, "kb": systats.Kilobyte,
		"MB": systats.Megabyte, " gb ": systats.Gigabyte,
	}
	for in, want := range cases {
		got, err := parseUnit(in)
		if err != nil {
			t.Errorf("parseUnit(%q) returned %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseUnit(%q) = %v, want %v", in, got, want)
		}
	}

	if _, err := parseUnit("furlongs"); err == nil {
		t.Errorf("parseUnit(furlongs) returned nil error")
	}
}
