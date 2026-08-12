// Package demo seeds realistic fake data for project_id "demo" so the
// dashboard has something to show without waiting on real traffic.
package demo

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"time"

	"vigil/internal/models"
)

const ProjectID = "demo"

const (
	rawWindow    = 24 * time.Hour
	rawInterval  = 8 * time.Second
	historyDays  = 7
	baselineGood = 0.95 // matches the 95% "fast" event bucket below

	breachAgo      = 48 * time.Hour
	breachDuration = 15 * time.Minute
	breachGoodRate = 0.92 // 8% error rate during the breach
)

// Store is the subset of storage.Store the seeder needs.
type Store interface {
	DeleteProjectData(projectID string) error
	InsertEventsBatch(events []models.Event) error
	InsertMetricsBatch(metrics []models.Metric) error
	InsertAlert(models.Alert) error
}

// Handler serves GET /demo.
func Handler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := Seed(store, time.Now()); err != nil {
			log.Printf("demo seed: %v", err)
			http.Error(w, "seed failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"seeded":     true,
			"project_id": ProjectID,
		})
	}
}

// Seed wipes any existing "demo" project data and regenerates it, so
// calling it more than once doesn't pile up duplicate history.
func Seed(store Store, now time.Time) error {
	if err := store.DeleteProjectData(ProjectID); err != nil {
		return err
	}

	rng := rand.New(rand.NewSource(now.UnixNano()))

	if err := seedRawEvents(store, rng, now); err != nil {
		return err
	}
	if err := seedAggregatedHistory(store, rng, now); err != nil {
		return err
	}
	return seedBreachIncident(store, now)
}

// seedRawEvents fills the last 24h with individual events, since raw_events
// is where per-event latency lives and both the badge and the dashboard's
// latency chart read from it.
func seedRawEvents(store Store, rng *rand.Rand, now time.Time) error {
	start := now.Add(-rawWindow)
	events := make([]models.Event, 0, int(rawWindow/rawInterval))
	for ts := start; ts.Before(now); ts = ts.Add(rawInterval) {
		events = append(events, syntheticEvent(rng, ts))
	}
	return store.InsertEventsBatch(events)
}

// seedAggregatedHistory fills days 2-7 ago as already-rolled-up hourly
// buckets, mirroring what Downsample would have produced by now for data
// that old — raw_events only ever holds the last 24h.
func seedAggregatedHistory(store Store, rng *rand.Rand, now time.Time) error {
	start := now.Add(-historyDays * 24 * time.Hour).Truncate(time.Hour)
	end := now.Add(-rawWindow).Truncate(time.Hour)
	breachBucket := now.Add(-breachAgo).Truncate(time.Hour)

	var metrics []models.Metric
	for b := start; b.Before(end); b = b.Add(time.Hour) {
		total := int64(200 + rng.Intn(200)) // 200-400 events/hour, some variation
		goodRate := baselineGood
		if b.Equal(breachBucket) {
			goodRate = blendedGoodRate()
		}
		metrics = append(metrics, models.Metric{
			ProjectID:   ProjectID,
			BucketStart: b,
			Resolution:  "1h",
			GoodEvents:  int64(float64(total) * goodRate),
			TotalEvents: total,
		})
	}
	return store.InsertMetricsBatch(metrics)
}

// blendedGoodRate models one hour where 15 minutes ran at breachGoodRate
// and the rest ran at the normal baseline.
func blendedGoodRate() float64 {
	breachFraction := float64(breachDuration) / float64(time.Hour)
	return breachFraction*breachGoodRate + (1-breachFraction)*baselineGood
}

// seedBreachIncident records the fake, already-resolved alert for the
// simulated breach 2 days ago.
func seedBreachIncident(store Store, now time.Time) error {
	bucket := now.Add(-breachAgo).Truncate(time.Hour)
	firedAt := bucket.Add(20 * time.Minute) // arbitrary offset within the bucket
	resolvedAt := firedAt.Add(breachDuration)

	return store.InsertAlert(models.Alert{
		ProjectID:         ProjectID,
		FiredAt:           firedAt,
		ResolvedAt:        resolvedAt,
		BudgetConsumedPct: 62.4, // representative snapshot for the demo
		SLOPct:            89.3,
		TargetPct:         90.0, // matches the "demo" SLO in vigil.yaml.example
		ShortBurnRate:     18.2,
		LongBurnRate:      3.6,
	})
}

func syntheticEvent(rng *rand.Rand, ts time.Time) models.Event {
	u := rng.Float64()
	switch {
	case u < 0.95:
		return models.Event{
			ProjectID: ProjectID, Timestamp: ts,
			LatencyMS: randRange(rng, 80, 180), StatusCode: 200, Error: false,
		}
	case u < 0.99:
		return models.Event{
			ProjectID: ProjectID, Timestamp: ts,
			LatencyMS: randRange(rng, 200, 400), StatusCode: 200, Error: false,
		}
	default:
		return models.Event{
			ProjectID: ProjectID, Timestamp: ts,
			LatencyMS: randRange(rng, 500, 2000), StatusCode: 500, Error: true,
		}
	}
}

func randRange(rng *rand.Rand, lo, hi int64) int64 {
	return lo + rng.Int63n(hi-lo+1)
}
