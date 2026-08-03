const SHORT_THRESHOLD = 14.4;
const LONG_THRESHOLD = 1.0;

function severity(value, threshold) {
  const ratio = value / threshold;
  if (ratio >= 1) return { fill: "#d03b3b", track: "#f3d9d9", label: "breaching" };
  if (ratio >= 0.7) return { fill: "#fab219", track: "#fcebc7", label: "elevated" };
  return { fill: "#0ca30c", track: "#cdeccd", label: "normal" };
}

function Meter({ title, window, value, threshold }) {
  const { fill, track, label } = severity(value, threshold);
  // Scale the bar to 2x threshold so "at threshold" reads as the midpoint.
  const pct = Math.max(0, Math.min(100, (value / (threshold * 2)) * 100));

  return (
    <div className="rounded-lg border border-[#e1e0d9] dark:border-[#2c2c2a] bg-[#fcfcfb] dark:bg-[#1a1a19] p-5 flex-1">
      <div className="flex items-baseline justify-between">
        <div>
          <div className="text-sm font-medium text-[#0b0b0b] dark:text-white">{title}</div>
          <div className="text-xs text-[#898781]">{window} window</div>
        </div>
        <div className="text-right">
          <div className="text-2xl font-semibold" style={{ color: fill }}>
            {value.toFixed(2)}x
          </div>
          <div className="text-xs uppercase tracking-wide" style={{ color: fill }}>
            {label}
          </div>
        </div>
      </div>
      <div
        className="mt-3 h-2.5 w-full overflow-hidden rounded-full relative"
        style={{ backgroundColor: track }}
        role="meter"
        aria-valuenow={value}
        aria-valuemin={0}
        aria-valuemax={threshold * 2}
      >
        <div
          className="h-full rounded-full transition-[width]"
          style={{ width: `${pct}%`, backgroundColor: fill }}
        />
        <div
          className="absolute top-0 h-full w-px bg-[#52514e] dark:bg-[#c3c2b7]"
          style={{ left: "50%" }}
          title={`threshold ${threshold}x`}
        />
      </div>
      <div className="mt-1 text-xs text-[#898781]">alert threshold {threshold}x</div>
    </div>
  );
}

export default function BurnRateMeters({ slo }) {
  return (
    <div className="flex flex-col sm:flex-row gap-4">
      <Meter
        title="Short window burn rate"
        window="5m"
        value={slo.short_burn_rate}
        threshold={SHORT_THRESHOLD}
      />
      <Meter
        title="Long window burn rate"
        window="1h"
        value={slo.long_burn_rate}
        threshold={LONG_THRESHOLD}
      />
    </div>
  );
}
