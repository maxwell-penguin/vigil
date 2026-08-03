package collector

import (
	"log"
	"net/http"
	"time"

	"vigil/internal/models"
)

// Prober polls each target on an interval and stores an event per probe.
type Prober struct {
	Targets  []models.ProbeTarget
	Store    Ingester
	Interval time.Duration
	Client   *http.Client
}

func NewProber(targets []models.ProbeTarget, store Ingester, interval time.Duration) *Prober {
	return &Prober{
		Targets:  targets,
		Store:    store,
		Interval: interval,
		Client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *Prober) Run(stop <-chan struct{}) {
	if len(p.Targets) == 0 {
		log.Printf("prober: no targets configured, exiting loop")
		return
	}
	t := time.NewTicker(p.Interval)
	defer t.Stop()
	p.probeAll()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			p.probeAll()
		}
	}
}

func (p *Prober) probeAll() {
	for _, tgt := range p.Targets {
		p.probeOne(tgt)
	}
}

func (p *Prober) probeOne(tgt models.ProbeTarget) {
	start := time.Now()
	status := 0
	transportErr := false

	req, err := http.NewRequest(http.MethodGet, tgt.URL, nil)
	if err != nil {
		log.Printf("prober %s: bad url: %v", tgt.ProjectID, err)
		return
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		transportErr = true
		log.Printf("prober %s: %v", tgt.ProjectID, err)
	} else {
		status = resp.StatusCode
		resp.Body.Close()
	}
	latency := time.Since(start).Milliseconds()

	isError := transportErr || status != tgt.ExpectedStatus
	if err := p.Store.InsertEvent(models.Event{
		ProjectID:  tgt.ProjectID,
		Timestamp:  start.UTC(),
		LatencyMS:  latency,
		StatusCode: status,
		Error:      isError,
	}); err != nil {
		log.Printf("prober %s: store: %v", tgt.ProjectID, err)
	}
}
