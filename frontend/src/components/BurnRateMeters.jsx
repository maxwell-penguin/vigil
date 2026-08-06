const SHORT_THRESHOLD = 14.4;
const LONG_THRESHOLD = 1.0;

function severity(ratio) {
  if (ratio >= 1) return "#dc2626";
  if (ratio >= 0.7) return "#d97706";
  return "#16a34a";
}

function Row({ label, value, threshold }) {
  const ratio = value / threshold;
  const breaching = ratio >= 1;
  const color = severity(ratio);

  return (
    <div className="flex items-center gap-4 py-3">
      <div className="w-24 shrink-0 text-sm text-ink-secondary">{label}</div>

      <div
        className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-line"
        role="meter"
        aria-label={`${label} burn rate`}
        aria-valuenow={value}
        aria-valuemin={0}
        aria-valuemax={threshold}
      >
        <div
          className="h-full rounded-full transition-[width]"
          style={{
            width: `${Math.max(0, Math.min(100, ratio * 100))}%`,
            backgroundColor: color,
          }}
        />
      </div>

      <div className="w-16 shrink-0 text-right font-mono text-sm" style={{ color }}>
        {value.toFixed(2)}×
      </div>

      <span
        className={
          "w-24 shrink-0 rounded-md px-2 py-0.5 text-center text-xs font-medium " +
          (breaching ? "bg-danger-soft text-danger" : "bg-ok-soft text-ok")
        }
      >
        {breaching ? "BREACHING" : "OK"}
      </span>
    </div>
  );
}

export default function BurnRateMeters({ slo }) {
  return (
    <div className="card p-5">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <div className="text-sm font-medium text-ink">Burn rate monitor</div>
        <div className="text-xs text-ink-muted">
          Alerts when both windows exceed threshold
        </div>
      </div>

      <div className="mt-2 divide-y divide-line">
        <Row label="5 min window" value={slo.short_burn_rate} threshold={SHORT_THRESHOLD} />
        <Row label="1 hr window" value={slo.long_burn_rate} threshold={LONG_THRESHOLD} />
      </div>

      <div className="mt-3 border-t border-line pt-3 text-xs text-ink-muted">
        Alert threshold: {SHORT_THRESHOLD}× (5 min) · {LONG_THRESHOLD}× (1 hr) — both
        windows must exceed threshold to fire
      </div>
    </div>
  );
}
