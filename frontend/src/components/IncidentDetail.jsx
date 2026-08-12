const GITHUB_REPO = import.meta.env.VITE_GITHUB_REPO;

// Longhand duration for the summary header — distinct from the table row's
// compact "15m" so the detail view reads like prose ("15 minutes").
function formatDurationLong(sec) {
  if (!sec || sec <= 0) return "—";
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const parts = [];
  if (h > 0) parts.push(`${h} hour${h === 1 ? "" : "s"}`);
  if (m > 0 || h === 0) parts.push(`${m} minute${m === 1 ? "" : "s"}`);
  return parts.join(" ");
}

function formatTime(iso) {
  if (!iso) return "—";
  return new Date(iso).toLocaleString();
}

function postmortemText(inc) {
  const detected = formatTime(inc.fired_at);
  const duration = inc.ongoing ? "Ongoing" : formatDurationLong(inc.breach_duration_sec);
  const timeline = [`- ${detected} — SLO breach detected by Vigil`];
  if (!inc.ongoing) {
    timeline.push(`- ${formatTime(inc.resolved_at)} — Incident resolved`);
  }

  return `## Incident Report
**Detected:** ${detected}
**Duration:** ${duration}
**SLO Impact:** ${inc.slo_pct.toFixed(2)}% (target ${inc.target_pct.toFixed(1)}%)
**Error Budget Consumed:** ${inc.budget_consumed_pct.toFixed(1)}%

### Impact
_To be filled in_

### Root Cause
_To be filled in_

### Timeline
${timeline.join("\n")}

### Action Items
- [ ] _Add action items_`;
}

export default function IncidentDetail({ incident }) {
  return (
    <div
      className="font-sans text-sm text-ink"
      style={{ background: "#f9fafb", borderLeft: "3px solid #dc2626", padding: 20 }}
    >
      <div className="flex flex-wrap items-center gap-3">
        <span
          className={
            "rounded-full px-2 py-0.5 text-xs font-semibold tracking-wide " +
            (incident.ongoing ? "bg-danger-soft text-danger" : "bg-ok-soft text-ok")
          }
        >
          {incident.ongoing ? "ONGOING" : "RESOLVED"}
        </span>
        <span className="text-ink-secondary">
          Duration: <span className="text-ink">{formatDurationLong(incident.breach_duration_sec)}</span>
        </span>
        <span className="text-ink-secondary">
          Budget consumed:{" "}
          <span className="text-ink">{incident.budget_consumed_pct.toFixed(1)}% of error budget</span>
        </span>
        <span className="text-ink-secondary">
          Detected at: <span className="font-mono text-ink">{formatTime(incident.fired_at)}</span>
        </span>
      </div>

      <div className="mt-4 flex flex-wrap gap-6">
        <div>
          <div className="text-xs uppercase tracking-wider text-ink-muted">Short window burn rate</div>
          <div className="mt-1 font-mono text-lg font-semibold text-danger">
            {incident.short_burn_rate.toFixed(1)}x
          </div>
        </div>
        <div>
          <div className="text-xs uppercase tracking-wider text-ink-muted">Long window burn rate</div>
          <div className="mt-1 font-mono text-lg font-semibold text-danger">
            {incident.long_burn_rate.toFixed(1)}x
          </div>
        </div>
      </div>

      <pre
        className="mt-4 overflow-x-auto whitespace-pre-wrap"
        style={{
          background: "#1e1e1e",
          color: "#ffffff",
          fontFamily: "'JetBrains Mono', ui-monospace, monospace",
          padding: 16,
          borderRadius: 6,
        }}
      >
        {postmortemText(incident)}
      </pre>

      <div className="mt-4">
        {incident.issue_number ? (
          GITHUB_REPO ? (
            <a
              href={`https://github.com/${GITHUB_REPO}/issues/${incident.issue_number}`}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center rounded-md bg-ink px-3 py-1.5 text-xs font-medium text-white hover:bg-ink/90"
            >
              View GitHub Issue #{incident.issue_number}
            </a>
          ) : (
            <span className="text-xs text-ink-muted">#{incident.issue_number}</span>
          )
        ) : (
          <span className="text-xs text-ink-muted">No GitHub issue opened</span>
        )}
      </div>
    </div>
  );
}
