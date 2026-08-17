package slo

import (
	"log"
	"time"

	"vigil/internal/models"
	"vigil/internal/notify"
	"vigil/internal/selfmetrics"
)

// RecentEventsForPostmortem caps how many raw events a Notifier gets per breach.
const RecentEventsForPostmortem = 5

type Checker struct {
	Store           Store
	SLOs            []models.SLO
	Interval        time.Duration
	Notifier        Notifier                // optional; nil means log-only
	WebhookNotifier *notify.WebhookNotifier // optional; nil disables Slack/Discord alerts
}

func NewChecker(store Store, slos []models.SLO, interval time.Duration, notifier Notifier, webhookNotifier *notify.WebhookNotifier) *Checker {
	return &Checker{Store: store, SLOs: slos, Interval: interval, Notifier: notifier, WebhookNotifier: webhookNotifier}
}

func (c *Checker) Run(stop <-chan struct{}) {
	if len(c.SLOs) == 0 {
		log.Printf("slo checker: no SLOs configured, exiting loop")
		return
	}
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	c.checkAll(time.Now())
	for {
		select {
		case <-stop:
			return
		case now := <-t.C:
			c.checkAll(now)
		}
	}
}

func (c *Checker) checkAll(now time.Time) {
	selfmetrics.SLOChecksRun.Add(1)
	for _, s := range c.SLOs {
		st, err := Compute(c.Store, s, now)
		if err != nil {
			log.Printf("slo %s: compute: %v", s.ProjectID, err)
			continue
		}
		if !st.IsBreaching {
			c.resolveIfOpen(s.ProjectID, now)
			continue
		}
		last, ok, err := c.Store.LatestAlert(s.ProjectID)
		if err != nil {
			log.Printf("slo %s: latest alert: %v", s.ProjectID, err)
			continue
		}
		// now.Before(last.FiredAt) means the clock moved backward since the
		// last alert (NTP correction, container clock reset, a backfill run
		// with an old timestamp...). In that case now.Sub(last.FiredAt) is
		// negative and would always satisfy "< AlertCooldown", silently
		// extending suppression by however large the backward jump was. For
		// a paging system, failing toward an extra alert during a clock
		// anomaly is the safer failure mode than failing toward silence
		// during what might be a real, ongoing outage - so a backward jump
		// is never treated as "still in cooldown".
		inCooldown := ok && last.ResolvedAt.IsZero() && !now.Before(last.FiredAt) && now.Sub(last.FiredAt) < AlertCooldown
		if inCooldown {
			continue // still within cooldown of an unresolved alert
		}

		log.Printf("ALERT slo=%s slo_pct=%.3f short_burn=%.2f long_burn=%.2f",
			s.ProjectID, st.SLOPct, st.ShortBurnRate, st.LongBurnRate)

		alert := models.Alert{
			ProjectID:         s.ProjectID,
			FiredAt:           now,
			BudgetConsumedPct: st.BudgetConsumedPct,
			SLOPct:            st.SLOPct,
			TargetPct:         s.TargetPct,
			ShortBurnRate:     st.ShortBurnRate,
			LongBurnRate:      st.LongBurnRate,
		}

		if c.Notifier != nil {
			events, err := c.Store.FetchRecentRawEvents(s.ProjectID, RecentEventsForPostmortem)
			if err != nil {
				log.Printf("slo %s: fetch recent events: %v", s.ProjectID, err)
			}
			issueNumber, err := c.Notifier.NotifyBreach(s, st, events, now)
			if err != nil {
				log.Printf("slo %s: notify: %v", s.ProjectID, err)
			}
			alert.IssueNumber = issueNumber
		}

		// Independent of the GitHub notifier above: a failing webhook is
		// logged and never blocks the issue notification or the alert record.
		if c.WebhookNotifier != nil {
			if err := c.WebhookNotifier.Notify(alert, s.ProjectID); err != nil {
				log.Printf("slo %s: webhook notify: %v", s.ProjectID, err)
			}
		}

		if err := c.Store.InsertAlert(alert); err != nil {
			log.Printf("slo %s: insert alert: %v", s.ProjectID, err)
		}
	}
}

// resolveIfOpen closes out the latest alert once burn rates drop back below
// threshold, so /incidents can report a breach duration.
func (c *Checker) resolveIfOpen(projectID string, now time.Time) {
	last, ok, err := c.Store.LatestAlert(projectID)
	if err != nil {
		log.Printf("slo %s: latest alert: %v", projectID, err)
		return
	}
	if !ok || !last.ResolvedAt.IsZero() {
		return
	}
	if err := c.Store.MarkAlertResolved(projectID, last.FiredAt, now); err != nil {
		log.Printf("slo %s: mark resolved: %v", projectID, err)
	}
}
