// Package selfmetrics tracks vigil's own operational health — separate from
// the per-project metrics in internal/storage, which describe the projects
// vigil monitors, not vigil itself.
package selfmetrics

import "sync/atomic"

var (
	IngestEventsReceived atomic.Int64 // total events received on /ingest
	IngestEventsDropped  atomic.Int64 // dropped due to full write queue
	WriteQueueDepth      atomic.Int64 // current queue depth (set, not increment)
	WebhookFailures      atomic.Int64 // webhook delivery failures
	GitHubIssueFailures  atomic.Int64 // github issue creation failures
	SLOChecksRun         atomic.Int64 // total SLO check cycles completed

	RateLimiterTrackedProjects atomic.Int64 // current number of tracked per-project rate limiters
	RateLimiterEvictions       atomic.Int64 // total rate limiters evicted (LRU cap reached)
)
