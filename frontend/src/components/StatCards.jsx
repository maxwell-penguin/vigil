// ponytail: the /slo API returns measured values only — target_pct and
// latency_threshold_ms live in vigil.yaml and aren't served. These mirror the
// config defaults for display; read them from the API once it exposes them.
export const TARGET_PCT = 99.5;
export const LATENCY_TARGET_MS = 200;

const GREEN = "#16a34a";
const YELLOW = "#d97706";
const RED = "#dc2626";
const NEUTRAL = "#e5e7eb";

function latest(metrics) {
  return metrics.length ? metrics[metrics.length - 1] : null;
}

// Inline borderTop beats the .card:hover border-color rule, so the stripe
// keeps its colour on hover.
function Card({ label, value, sub, tone = "text-ink", stripe = NEUTRAL }) {
  return (
    <div className="card overflow-hidden p-4" style={{ borderTop: `2px solid ${stripe}` }}>
      <div className="text-xs font-medium uppercase tracking-wider text-ink-muted">
        {label}
      </div>
      <div className={`mt-2 font-mono text-[28px] leading-none ${tone}`}>{value}</div>
      <div className="mt-2 text-xs text-ink-secondary">{sub}</div>
    </div>
  );
}

export default function StatCards({ slo, metrics, incidents }) {
  const last = latest(metrics);
  const open = incidents.filter((i) => i.ongoing).length;

  const budgetTone =
    slo.budget_remaining_pct < 20
      ? "text-danger"
      : slo.budget_remaining_pct < 50
        ? "text-warn"
        : "text-ok";

  const p95Tone = !last
    ? "text-ink"
    : last.p95_ms <= LATENCY_TARGET_MS
      ? "text-ok"
      : last.p95_ms <= LATENCY_TARGET_MS * 1.5
        ? "text-warn"
        : "text-danger";

  const budgetStripe =
    slo.budget_remaining_pct < 5 ? RED : slo.budget_remaining_pct <= 20 ? YELLOW : GREEN;

  const p95Stripe = !last
    ? NEUTRAL
    : last.p95_ms < LATENCY_TARGET_MS
      ? GREEN
      : last.p95_ms <= LATENCY_TARGET_MS * 1.2
        ? YELLOW
        : RED;

  const incidentStripe =
    incidents.length === 0 ? GREEN : incidents.length <= 2 ? YELLOW : RED;

  return (
    <div id="overview" className="grid scroll-mt-20 grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card
        label="Uptime (30d)"
        value={`${slo.slo_pct.toFixed(2)}%`}
        sub={`Target ${TARGET_PCT}%`}
        tone={slo.slo_pct >= TARGET_PCT ? "text-ok" : "text-danger"}
        stripe={slo.slo_pct >= TARGET_PCT ? GREEN : RED}
      />
      <Card
        label="Budget remaining"
        value={`${slo.budget_remaining_pct.toFixed(1)}%`}
        sub={`${slo.budget_consumed_pct.toFixed(1)}% consumed`}
        tone={budgetTone}
        stripe={budgetStripe}
      />
      <Card
        label="p95 latency"
        value={last ? `${last.p95_ms}ms` : "—"}
        sub={`Target ${LATENCY_TARGET_MS}ms`}
        tone={p95Tone}
        stripe={p95Stripe}
      />
      <Card
        label="Incidents (30d)"
        value={incidents.length}
        sub={open > 0 ? `${open} open` : "None open"}
        tone={open > 0 ? "text-danger" : "text-ink"}
        stripe={incidentStripe}
      />
    </div>
  );
}

// Proactive warning shown between the two stat rows and the burn rate
// monitor — only when the current burn rate would exhaust the budget soon.
// Healthy/exhausted/not-burning states render nothing (exhausted already
// has its own "Outage"-style signal elsewhere; this banner is specifically
// the "you're about to breach" heads-up).
export function BurnTrajectoryBanner({ slo }) {
  const trajectory = slo.burn_trajectory;
  if (trajectory !== "warning" && trajectory !== "critical") return null;

  const critical = trajectory === "critical";
  const days = Math.round(slo.days_until_budget_exhausted);

  return (
    <div
      className={
        "rounded-md border px-4 py-3 text-sm font-medium " +
        (critical
          ? "border-[#fecaca] bg-[#fee2e2] text-[#991b1b]"
          : "border-[#fde68a] bg-[#fef3c7] text-[#92400e]")
      }
    >
      {critical
        ? `🔴 Error budget exhausts in ${days} days at current rate`
        : `⚠️ At current burn rate, error budget exhausts in ${days} days`}
    </div>
  );
}

function Bar({ pct, color }) {
  return (
    <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-line">
      <div
        className="h-full rounded-full transition-[width]"
        style={{ width: `${Math.max(0, Math.min(100, pct))}%`, backgroundColor: color }}
      />
    </div>
  );
}

// Row 2 — the two SLO detail cards. Same summary family as the stat grid, so
// they share this file rather than earning one of their own.
export function SLOCards({ slo, metrics }) {
  const budgetColor =
    slo.budget_remaining_pct < 20
      ? "#dc2626"
      : slo.budget_remaining_pct < 50
        ? "#d97706"
        : "#16a34a";

  // Latency compliance isn't served either — derive it from the buckets we
  // already have: share of buckets whose p95 met the target.
  const within = metrics.filter((m) => m.p95_ms <= LATENCY_TARGET_MS).length;
  const compliance = metrics.length ? (within / metrics.length) * 100 : null;
  const latencyOK = compliance === null || compliance >= TARGET_PCT;

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <div className="card p-5">
        <div className="flex items-baseline justify-between">
          <div className="text-sm font-medium text-ink">Availability SLO</div>
          <div className="text-xs text-ink-muted">30d window</div>
        </div>
        <div className="mt-3 flex items-baseline gap-2">
          <span
            className={`font-mono text-2xl leading-none ${
              slo.slo_pct >= TARGET_PCT ? "text-ok" : "text-danger"
            }`}
          >
            {slo.slo_pct.toFixed(2)}%
          </span>
          <span className="text-xs text-ink-secondary">of {TARGET_PCT}% target</span>
        </div>
        <Bar pct={slo.budget_remaining_pct} color={budgetColor} />
        <div className="mt-2 flex justify-between text-xs text-ink-secondary">
          <span>{slo.budget_consumed_pct.toFixed(1)}% budget consumed</span>
          <span className="font-mono">{slo.budget_remaining_pct.toFixed(1)}% left</span>
        </div>
      </div>

      <div className="card p-5">
        <div className="flex items-baseline justify-between">
          <div className="text-sm font-medium text-ink">Latency SLO</div>
          <div className="text-xs text-ink-muted">p95 &le; {LATENCY_TARGET_MS}ms</div>
        </div>
        <div className="mt-3 flex items-baseline gap-2">
          <span
            className={`font-mono text-2xl leading-none ${latencyOK ? "text-ok" : "text-warn"}`}
          >
            {compliance === null ? "—" : `${compliance.toFixed(1)}%`}
          </span>
          <span className="text-xs text-ink-secondary">of buckets within target</span>
        </div>
        <Bar pct={compliance ?? 0} color={latencyOK ? "#16a34a" : "#d97706"} />
        <div className="mt-2 flex justify-between text-xs text-ink-secondary">
          <span>
            {metrics.length ? `${metrics.length - within} of ${metrics.length} over target` : "No data"}
          </span>
          <span className={latencyOK ? "text-ok" : "text-warn"}>
            {latencyOK ? "Healthy" : "Elevated"}
          </span>
        </div>
      </div>
    </div>
  );
}
