package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/dhamith93/systats"
)

// The palette. Kept here rather than in CSS because the gauge colours are
// chosen per-value in Go, not by a stylesheet rule.
const (
	colorOK   = "#2f9e44"
	colorWarn = "#e8a317"
	colorCrit = "#d64545"

	colorEstablished = "#2f9e44"
	colorListen      = "#3b82c4"
	colorTimeWait    = "#e8a317"
	colorCloseWait   = "#d64545"
	colorOther       = "#8a8f98"
	colorUnknown     = "#6b4fa0"
)

// Gauge geometry. The donut is drawn as a circle whose stroke-dasharray
// splits into a coloured arc and a transparent remainder, rotated so the
// arc starts at 12 o'clock.
const (
	gaugeRadius = 54.0
	gaugeSize   = 140.0
)

// Gauge holds a donut's pre-computed SVG values. html/template can't do
// arithmetic, so everything the markup needs is resolved here.
type Gauge struct {
	Percent       float64
	Dash          string // filled arc length
	Gap           string // remainder of the circumference
	Circumference string
	Color         string
	Label         string // big text in the middle
	Caption       string // small text under it
	Radius        string
	Center        string
	Size          string
}

// Bar is a horizontal meter: a track with a filled portion.
type Bar struct {
	Percent float64
	Width   string // clamped percentage, as a CSS width
	Color   string
	Label   string
	Value   string
}

// Segment is one slice of the stacked TCP-state bar.
type Segment struct {
	Label   string
	Percent float64
	Width   string
	Color   string
}

// dashArray converts a percentage into the filled and unfilled lengths of
// a circle's stroke-dasharray. Values outside 0-100 are clamped: a CPU
// reading slightly over 100 would otherwise wrap the arc past its start
// and render as a nearly-empty ring.
func dashArray(pct, radius float64) (dash, gap float64) {
	circumference := 2 * math.Pi * radius
	dash = circumference * clampPercent(pct) / 100
	return dash, circumference - dash
}

// clampPercent bounds a percentage to 0-100 and maps NaN to 0. NaN is
// reachable: a zero-sized filesystem or a zero elapsed interval makes the
// division that produced this value undefined.
func clampPercent(pct float64) float64 {
	if math.IsNaN(pct) || pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// levelColor maps a utilisation percentage to the palette. The thresholds
// are the conventional ones for capacity dashboards - comfortable below
// 70, worth noticing by 85.
func levelColor(pct float64) string {
	switch {
	case clampPercent(pct) >= 85:
		return colorCrit
	case clampPercent(pct) >= 70:
		return colorWarn
	default:
		return colorOK
	}
}

// tempColor prefers the chip's own thresholds over invented ones. A
// sensor that publishes no _max/_crit files reports zero for them, and
// treating those zeros as real limits would paint every reading red - so
// the flags decide, not the values.
func tempColor(t systats.Temperature) string {
	switch {
	case t.CriticalAvailable && t.Celsius >= t.Critical:
		return colorCrit
	case t.HighAvailable && t.Celsius >= t.High:
		return colorWarn
	case !t.HighAvailable && !t.CriticalAvailable:
		// No thresholds published (common on ARM boards). Fall back to a
		// 100°C ceiling, which makes degrees and percent coincide - so
		// warn lands at 70°C and critical at 85°C.
		return levelColor(t.Celsius)
	default:
		return colorOK
	}
}

// tempScale turns a reading into a gauge percentage plus a caption. With
// a critical threshold the scale is relative to it; without one, 100°C
// stands in as a reasonable ceiling for silicon.
func tempScale(t systats.Temperature) (pct float64, limits string) {
	ceiling := 100.0
	switch {
	case t.CriticalAvailable && t.Critical > 0:
		ceiling = t.Critical
		limits = fmt.Sprintf("crit %.0f°C", t.Critical)
		if t.HighAvailable && t.High > 0 {
			limits = fmt.Sprintf("high %.0f°C, crit %.0f°C", t.High, t.Critical)
		}
	case t.HighAvailable && t.High > 0:
		ceiling = t.High
		limits = fmt.Sprintf("high %.0f°C", t.High)
	default:
		limits = "no thresholds published"
	}
	if ceiling <= 0 {
		ceiling = 100
	}
	return 100 * t.Celsius / ceiling, limits
}

func newGauge(pct float64, label, caption string) Gauge {
	return newGaugeColored(pct, label, caption, levelColor(pct))
}

func newGaugeColored(pct float64, label, caption, color string) Gauge {
	dash, gap := dashArray(pct, gaugeRadius)
	return Gauge{
		Percent:       clampPercent(pct),
		Dash:          fmtFloat(dash),
		Gap:           fmtFloat(gap),
		Circumference: fmtFloat(2 * math.Pi * gaugeRadius),
		Color:         color,
		Label:         label,
		Caption:       caption,
		Radius:        fmtFloat(gaugeRadius),
		Center:        fmtFloat(gaugeSize / 2),
		Size:          fmtFloat(gaugeSize),
	}
}

func newBar(pct float64, label, value string) Bar {
	return Bar{
		Percent: clampPercent(pct),
		Width:   fmtFloat(clampPercent(pct)),
		Color:   levelColor(pct),
		Label:   label,
		Value:   value,
	}
}

// fmtFloat trims trailing zeros so the generated SVG stays readable when
// someone views source - the whole page is meant to be inspectable.
func fmtFloat(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "0"
	}
	s := strconv.FormatFloat(f, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// size renders a value already expressed in unit. systats returns these
// as float64 precisely so the larger units stay usable - 3.82 GB rather
// than a truncated 3.
func size(v float64, unit systats.Unit) string {
	return fmt.Sprintf("%.2f %s", v, unit)
}

// bytesHuman scales a raw byte count to a binary unit, matching what the
// library's own unit constants mean (MB is MiB).
func bytesHuman(b float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0
	for b >= 1024 && i < len(units)-1 {
		b /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", b, units[i])
	}
	return fmt.Sprintf("%.2f %s", b, units[i])
}

func rate(bytesPerSec float64) string {
	if bytesPerSec <= 0 {
		return "idle"
	}
	return bytesHuman(bytesPerSec) + "/s"
}
