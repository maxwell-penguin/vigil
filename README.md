# vigil

**SRE reliability toolkit for open source maintainers.**

Vigil tracks your project's uptime and latency, computes a real error-budget
SLO, catches breaches early with a two-window burn-rate alarm, and opens a
GitHub issue with a pre-filled postmortem so you don't have to build any of
that yourself.

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
[![vigil status](https://vigil.your-domain.com/badge/my-project)](https://vigil.your-domain.com)
```

Renders green with your current SLO % when healthy, red with `BREACHING`
when you're not.

## Quick start

```sh
# 1. Clone
git clone https://github.com/your-org/vigil.git && cd vigil

# 2. Configure — set your probe targets and SLOs
cp vigil.yaml.example vigil.yaml

# 3. Run
go run ./cmd/server

# 4. Add the badge above to your README, pointed at your vigil server
```

Want to see it working before you have real traffic? `curl` the seed
endpoint and point the dashboard at project `demo`:

```sh
curl http://localhost:8080/demo
cd frontend && cp .env.example .env && npm install && npm run dev
```

Or run the whole thing — server, dashboard, and a self-instrumented demo
app — with `docker-compose up`.

## How breach detection works

Vigil uses the two-window burn-rate algorithm from the [Google SRE
Workbook](https://sre.google/workbook/alerting-on-slos/). Instead of
alerting the instant one request fails, it tracks how fast you're burning
through your 30-day error budget over two windows at once — the last 5
minutes and the last hour — and only fires when **both** are burning
dangerously fast (more than 14.4x and 1x the sustainable rate, respectively).
Requiring the fast, noisy signal and the slower, stable one to agree means a
single blip doesn't page you, but a real outage still gets caught within
minutes instead of after it's already eaten your whole month's budget.

## Project structure

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
