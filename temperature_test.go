package systats

import "testing"

func hwmonFixtureStats() *SyStats {
	return &SyStats{HwmonPath: "./test_files/hwmon"}
}

func TestGetTemperatures(t *testing.T) {
	got, err := getTemperatures(hwmonFixtureStats())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 4 temp*_input files exist, but hwmon1/temp2_input holds garbage and
	// must be skipped rather than parsed into a bogus reading.
	if len(got) != 3 {
		t.Fatalf("got %d readings, want 3: %+v", len(got), got)
	}

	// Sorted by chip name then label: coretemp before cpu_thermal.
	if got[0].Name != "coretemp" || got[0].Label != "Core 0" {
		t.Errorf("first reading = %+v, want coretemp/Core 0", got[0])
	}
	if got[0].Celsius != 43.5 {
		t.Errorf("Core 0 = %v C, want 43.5 (43500 millidegrees)", got[0].Celsius)
	}

	if got[1].Label != "Package id 0" {
		t.Errorf("second reading label = %q, want %q", got[1].Label, "Package id 0")
	}
	if got[1].Celsius != 45 {
		t.Errorf("Package id 0 = %v C, want 45", got[1].Celsius)
	}
	if !got[1].HighAvailable || got[1].High != 80 {
		t.Errorf("Package id 0 high = %v (available %v), want 80 available", got[1].High, got[1].HighAvailable)
	}
	if !got[1].CriticalAvailable || got[1].Critical != 100 {
		t.Errorf("Package id 0 critical = %v (available %v), want 100 available", got[1].Critical, got[1].CriticalAvailable)
	}
}

// A chip with no *_label falls back to the sensor's file prefix, and
// missing thresholds must be flagged rather than reported as 0 C - which
// would make every reading look critically over-limit.
func TestGetTemperaturesWithoutLabelsOrThresholds(t *testing.T) {
	got, err := getTemperatures(hwmonFixtureStats())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var pi *Temperature
	for i := range got {
		if got[i].Name == "cpu_thermal" {
			pi = &got[i]
		}
	}
	if pi == nil {
		t.Fatalf("no cpu_thermal reading in %+v", got)
	}

	if pi.Label != "temp1" {
		t.Errorf("Label = %q, want the %q file prefix when no _label exists", pi.Label, "temp1")
	}
	if pi.Celsius != 52.641 {
		t.Errorf("Celsius = %v, want 52.641", pi.Celsius)
	}
	if pi.HighAvailable {
		t.Errorf("HighAvailable = true, want false when no _max file exists")
	}
	if pi.CriticalAvailable {
		t.Errorf("CriticalAvailable = true, want false when no _crit file exists")
	}
	if pi.High != 0 || pi.Critical != 0 {
		t.Errorf("thresholds = %v/%v, want 0 when unavailable", pi.High, pi.Critical)
	}
}

// Sensors that publish only _max keep HighAvailable while leaving
// CriticalAvailable false.
func TestGetTemperaturesPartialThresholds(t *testing.T) {
	got, err := getTemperatures(hwmonFixtureStats())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !got[0].HighAvailable || got[0].High != 80 {
		t.Errorf("Core 0 high = %v (available %v), want 80 available", got[0].High, got[0].HighAvailable)
	}
	if got[0].CriticalAvailable {
		t.Errorf("Core 0 CriticalAvailable = true, want false - it has no _crit file")
	}
}

// Most VMs and containers have no hwmon directory at all. That's normal,
// not an error.
func TestGetTemperaturesNoHwmon(t *testing.T) {
	got, err := getTemperatures(&SyStats{HwmonPath: "./test_files/nonexistent_hwmon"})
	if err != nil {
		t.Fatalf("a host without hwmon should not be an error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d readings, want none", len(got))
	}
}

func TestTempInputPrefix(t *testing.T) {
	cases := map[string]string{
		"temp1_input":  "temp1",
		"temp12_input": "temp12",
	}
	for fileName, want := range cases {
		got, ok := tempInputPrefix(fileName)
		if !ok || got != want {
			t.Errorf("tempInputPrefix(%q) = %q/%v, want %q/true", fileName, got, ok, want)
		}
	}

	for _, fileName := range []string{"temp1_label", "temp1_max", "name", "in0_input", "temp_input", "fan1_input"} {
		if _, ok := tempInputPrefix(fileName); ok {
			t.Errorf("tempInputPrefix(%q) = ok, want it skipped", fileName)
		}
	}
}
