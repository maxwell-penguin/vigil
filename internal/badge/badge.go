package badge

import (
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	"vigil/internal/models"
	"vigil/internal/slo"
)

// Store is what the badge handler needs: SLO computation plus a recent p99.
type Store interface {
	slo.Store
	LatencyPercentile(projectID string, from, to time.Time, p float64) (int64, error)
}

const (
	colorGood     = "#0ca30c" // matches dataviz status palette, keeps badge and dashboard in sync
	colorCritical = "#d03b3b"
	label         = "vigil"
)

// svgTmpl is a flat, shields.io-style two-segment badge: a fixed gray label
// segment and a colored message segment.
var svgTmpl = template.Must(template.New("badge").Parse(`<svg xmlns="http://www.w3.org/2000/svg" height="20" width="{{.Width}}" role="img" aria-label="{{.Label}}: {{.Message}}">
  <linearGradient id="s" x2="0" y2="100%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <mask id="m">
    <rect width="{{.Width}}" height="20" rx="3" fill="#fff"/>
  </mask>
  <g mask="url(#m)">
    <rect width="{{.LabelWidth}}" height="20" fill="#555"/>
    <rect x="{{.LabelWidth}}" width="{{.MsgWidth}}" height="20" fill="{{.Color}}"/>
    <rect width="{{.Width}}" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
    <text x="{{.LabelX}}" y="15" fill="#010101" fill-opacity=".3">{{.Label}}</text>
    <text x="{{.LabelX}}" y="14">{{.Label}}</text>
    <text x="{{.MsgX}}" y="15" fill="#010101" fill-opacity=".3">{{.Message}}</text>
    <text x="{{.MsgX}}" y="14">{{.Message}}</text>
  </g>
</svg>
`))

type badgeData struct {
	Label, Message, Color       string
	Width, LabelWidth, MsgWidth int
	LabelX, MsgX                int
}

// ponytail: no font-metrics dependency — approximate 7px/char at this font
// size plus 10px padding per side, same rule of thumb shields.io badges use.
func textWidth(s string) int {
	return len(s)*7 + 20
}

func render(message, color string) badgeData {
	lw := textWidth(label)
	mw := textWidth(message)
	return badgeData{
		Label: label, Message: message, Color: color,
		Width: lw + mw, LabelWidth: lw, MsgWidth: mw,
		LabelX: lw / 2, MsgX: lw + mw/2,
	}
}

// Handler serves GET /badge/{project_id}: a live SVG badge for the project's
// current SLO status, with the last 5-minute p99 latency alongside it.
func Handler(store Store, slos []models.SLO) http.HandlerFunc {
	byID := make(map[string]models.SLO, len(slos))
	for _, s := range slos {
		byID[s.ProjectID] = s
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("project_id")
		s, ok := byID[id]
		if !ok {
			http.Error(w, "no SLO defined for project", http.StatusNotFound)
			return
		}

		now := time.Now()
		st, err := slo.Compute(store, s, now)
		if err != nil {
			log.Printf("badge %s: compute: %v", id, err)
			http.Error(w, "compute failed", http.StatusInternalServerError)
			return
		}
		p99, err := store.LatencyPercentile(id, now.Add(-slo.ShortWindow), now, 99)
		if err != nil {
			log.Printf("badge %s: p99: %v", id, err)
		}

		var message, color string
		if st.IsBreaching {
			message = fmt.Sprintf("BREACHING | p99 %dms", p99)
			color = colorCritical
		} else {
			message = fmt.Sprintf("%.1f%% | p99 %dms", st.SLOPct, p99)
			color = colorGood
		}

		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "no-cache")
		if err := svgTmpl.Execute(w, render(message, color)); err != nil {
			log.Printf("badge %s: render: %v", id, err)
		}
	}
}
