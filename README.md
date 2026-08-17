<div align="center">

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)
[![CI](https://github.com/maxwell-penguin/vigil/actions/workflows/test.yml/badge.svg)](https://github.com/maxwell-penguin/vigil/actions/workflows/test.yml)
![License](https://img.shields.io/badge/license-MIT-green)
![Live Demo](https://img.shields.io/badge/demo-live-brightgreen)
![Fly.io](https://img.shields.io/badge/deployed-fly.io-purple)

# Vigil

*SRE reliability toolkit for open source maintainers*

🚀 **[Live Demo](https://vigil-murex-ten.vercel.app)** — type `demo` to see 7 days of data including a simulated breach event

</div>

Vigil tracks your project's uptime and latency, computes a real error-budget SLO, catches breaches early with a two-window burn-rate alarm, and opens a GitHub issue with a pre-filled postmortem so you don't have to build any of that yourself.

## Features

- 📡 Push + pull metric collection (SDK batching + external prober)
- 📊 Two-window burn rate SLO engine (Google SRE Workbook algorithm)
- 🚨 Auto-opens GitHub Issues with pre-filled postmortem on breach
- ⏱️ SQLite time-series with automatic 1m/1h downsampling
- 🏷️ Embeddable SVG README badge showing live uptime
- 📈 Prometheus-compatible /prometheus/:project_id scrape endpoint
- 📦 Python + JavaScript SDKs with background batching
- 🐳 Docker + Fly.io ready, ~9MB image

## Demo

<!-- Add GIF demo here -->

Drop a live status badge in your own README:

```md
[![vigil status](https://vigil-calm-cherry-433.fly.dev/badge/example-site)](https://vigil-calm-cherry-433.fly.dev)
```

## Table of Contents

- [Features](#features)
- [Demo](#demo)
- [Quick Start](#quick-start)
- [How Breach Detection Works](#how-breach-detection-works)
- [Architecture](#architecture)
- [Badge](#badge)
- [Status Pages](#status-pages)
- [Deployment](#deployment)
- [Security](#security)
- [Backpressure and Load Shedding](#backpressure-and-load-shedding)
- [Project Structure](#project-structure)
- [Contributing](#contributing)
- [License](#license)

## Quick Start

```sh
# 1. Clone
git clone https://github.com/maxwell-penguin/vigil.git && cd vigil

# 2. Configure — set your probe targets and SLOs
cp vigil.yaml.example vigil.yaml

# 3. Run
go run ./cmd/server

# 4. Seed some demo data (7 days incl. a simulated breach) and see it live
curl http://localhost:8080/demo
cd frontend && cp .env.example .env && npm install && npm run dev

# 5. Add the badge above to your README, pointed at your vigil server
```

Or run the whole thing — server, dashboard, and a self-instrumented demo app — with `docker-compose up`.

## How Breach Detection Works

Vigil uses the two-window burn-rate algorithm from the [Google SRE
Workbook](https://sre.google/workbook/alerting-on-slos/). Instead of
alerting the instant one request fails, it tracks how fast you're burning
through your 30-day error budget over two windows at once — the last 5
minutes and the last hour — and only fires when **both** are burning
dangerously fast (more than 14.4x and 1x the sustainable rate, respectively).
Requiring the fast, noisy signal and the slower, stable one to agree means a
single blip doesn't page you, but a real outage still gets caught within
minutes instead of after it's already eaten your whole month's budget.

## Architecture

```
  Python/JS SDK   ─┐
                    ├─▶  POST /ingest  ─▶  Vigil Server (Go, :8080)
  External Prober  ─┘                              │
                              ┌───────────────────┼────────────────────┐
                              ▼                    ▼                    ▼
                          SQLite              SLO Engine        GET /slo, /metrics,
                          storage          (burn-rate check)    /badge, /incidents
                                                  │                     │
                                                  ▼                     ▼
                                            GitHub Issues            Dashboard
                                             (on breach)           (React + Vite)
```

Two ways to get events in — push from your own code with the SDK, or let
vigil's built-in prober poll a URL every 30s — both land on the same
`/ingest` endpoint. Everything downstream (SQLite, the SLO engine, GitHub
issues, the dashboard) doesn't know or care which one produced the data.

## Badge

Drop this in your README — it's a live SVG, generated per-request from your
current SLO status, `Cache-Control: no-cache` so it's never stale:

```md
[![vigil status](https://vigil-calm-cherry-433.fly.dev/badge/my-project)](https://vigil-calm-cherry-433.fly.dev)
```

Renders green with your current SLO % when healthy, red with `BREACHING`
when you're not.

## Status Pages

Every project gets a public status page at:

```
https://your-vigil-instance.fly.dev/status/{project_id}
```

No login required. Shows current operational status, 30-day uptime,
90-day uptime grid, and recent incident history. Link it from your
project's docs or README so users always know where to check.

Live example: https://vigil-calm-cherry-433.fly.dev/status/demo

## Deployment

- **Backend:** Fly.io — https://vigil-calm-cherry-433.fly.dev
- **Frontend:** Vercel — https://vigil-murex-ten.vercel.app

SQLite is persisted on a Fly volume. The Docker image is a static musl
build on `scratch`, ~9MB.

```sh
fly launch
fly volumes create vigil_data --size 1
flyctl deploy
```

Vigil monitors itself at GET /prometheus/vigil-internal — queue depth,
dropped events, webhook failures, and SLO check cycles in Prometheus format.

## Security

Set `api_key` in `vigil.yaml` to protect all endpoints except
the public badge and status page:

```yaml
api_key: "your-secret-key-here"
```

Then pass it in SDK calls and API requests:
```
Authorization: Bearer your-secret-key-here
```

Generate a strong key with: `openssl rand -hex 32`

Rate limiting: `/ingest` is rate-limited to 100 events/second per project
(burst of 500) to prevent accidental SDK misconfiguration from overwhelming
the database.

## Backpressure and Load Shedding

Vigil is built to survive being overwhelmed, not just to work when things are
calm. There are two layers of protection on the write path.

First, the per-project rate limiter catches most abuse before it becomes a
problem. Each project is capped at 100 events/second with a burst of 500. A
single misbehaving SDK or a bad deploy loop gets rejected with 429s
immediately, before it touches the database.

Second, anything that gets through in aggregate hits a bounded internal write
queue. Many different projects can each stay under their own rate limit and
still add up to more than the queue can handle. When that queue fills up,
Vigil sheds load on purpose instead of crashing or silently dropping data. It
returns 503 with `Retry-After` when the queue is completely full, or a 200
with an honest `dropped` count in the response body when only part of a
batch got dropped. Either way, the caller knows exactly what happened.

### Load test results

Setup: local server, 10 concurrent `loadgen` processes, 50 workers each (500
total concurrent workers), spread across 10 different project_ids, sustained
for 10 seconds, all hitting POST /ingest.

| Metric | Value |
|---|---|
| Total requests sent | 562,545 |
| 429 (per-project rate limit) | 547,529 |
| 202 (fully accepted) | 12,800 |
| 200 (partially accepted, some dropped) | 874 |
| 503 (write queue fully overloaded) | 1,342 |
| Events accepted | 134,353 |
| Events dropped (queue full) | 15,807 |
| Client latency p50 | 2.8ms to 7.3ms |
| Client latency p95 | 24ms to 33ms |
| Client latency p99 | 54ms to 63ms |

The server stayed up the whole test. Zero crashes, zero unhandled errors.

Most of the traffic, 547,529 of 562,545 requests, never got past the rate
limiter, which is exactly what it's for. The write queue only had to absorb
what got through, and it shed about 10% of that (15,807 dropped out of
150,160 accepted plus dropped) instead of falling over.

One honest note on methodology: the before and after queue depth snapshots
mostly read 0. The write queue flushes every 500ms, faster than a single
snapshot can catch it mid-burst. The dropped counter is the reliable signal
here, not the depth snapshot. If you check this yourself, watch
`vigil_internal_ingest_dropped_total`, not queue depth.

### Reproducing it

Run the load generator:

```sh
go run ./tools/loadgen -url http://localhost:8080 -project loadtest -workers 50 -duration 10s -batch-size 10
```

A single process against one project_id will mostly just hit the rate
limiter, since 100 events/second per project is a low ceiling. To actually
stress the shared write queue, run multiple `loadgen` processes at once
against different `-project` values. Each one stays under its own
per-project limit, and the combined load lands on the queue.

## Project Structure

```
vigil/
├── cmd/server/          # HTTP server entrypoint
├── internal/
│   ├── badge/           # SVG status badge (GET /badge/:id)
│   ├── collector/       # push ingest (POST /ingest) + external prober
│   ├── demo/            # GET /demo — seeds fake data for project "demo"
│   ├── github/          # opens/dedupes incident issues on breach
│   ├── models/          # shared types (Event, SLO, Alert, Config, ...)
│   ├── slo/             # SLO compute, burn-rate checker, /slo & /incidents
│   └── storage/         # SQLite schema, raw + aggregated time series
├── frontend/            # React + Tailwind dashboard (Vite)
├── vigil-sdk/           # Python client (pip install vigil-sdk)
├── vigil-js/            # JS/TS client (npm install vigil-sdk)
├── demo/                # standalone demo app for docker-compose
├── vigil.yaml.example   # copy to vigil.yaml and edit
├── docker-compose.yml   # vigil server + demo-app, one command
└── Dockerfile           # static musl build -> scratch image
```

## Contributing

PRs welcome. Please open an issue first to discuss what you'd like to
change. Run `go test ./...` before submitting.

## License

MIT
</content>
