package main

import (
	_ "embed"
	"html/template"
	"io"
	"math"
	"time"
)

// The template lives in its own file rather than a backtick string so it
// stays editable with HTML tooling. go:embed puts it in the binary, which
// keeps the cross-compiled dashboard a single file to scp.
//
//go:embed dashboard.tmpl.html
var dashboardHTML string

var dashboardTemplate = template.Must(template.New("dashboard").Parse(dashboardHTML))

// page wraps a Dashboard with the render-time settings the template needs
// but the collector has no business knowing about.
type page struct {
	Dashboard
	Refresh        bool
	RefreshSeconds int
}

// render writes the dashboard. A non-zero refresh adds a meta refresh,
// used only in -serve mode - a snapshot file that reloaded itself would
// just flicker, since the data is frozen at write time.
func render(w io.Writer, d Dashboard, refresh time.Duration) error {
	p := page{Dashboard: d}
	if refresh > 0 {
		p.Refresh = true
		// Round up: a sub-second refresh would floor to 0, which the
		// browser reads as "reload immediately, forever".
		p.RefreshSeconds = int(math.Ceil(refresh.Seconds()))
		if p.RefreshSeconds < 1 {
			p.RefreshSeconds = 1
		}
	}
	return dashboardTemplate.Execute(w, p)
}
