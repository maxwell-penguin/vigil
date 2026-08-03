// Meter spec (dataviz skill): fill carries severity, track is a lighter step
// of the same ramp so the state reads across the whole bar.
function budgetSeverity(pct) {
  if (pct < 20) return { fill: "#d03b3b", track: "#f3d9d9" }; // critical
  if (pct < 50) return { fill: "#fab219", track: "#fcebc7" }; // warning
  return { fill: "#0ca30c", track: "#cdeccd" }; // good
}

export default function TopBar({ projectId, slo }) {
  const { fill, track } = budgetSeverity(slo.budget_remaining_pct);
  const pct = Math.max(0, Math.min(100, slo.budget_remaining_pct));

  return (
    <div className="rounded-lg border border-[#e1e0d9] dark:border-[#2c2c2a] bg-[#fcfcfb] dark:bg-[#1a1a19] p-6">
      <div className="flex flex-wrap items-end justify-between gap-6">
        <div>
          <div className="text-sm text-[#52514e] dark:text-[#c3c2b7]">{projectId}</div>
          <div
            className={
              "mt-1 font-semibold " +
              (slo.is_breaching ? "text-[#d03b3b]" : "text-[#0b0b0b] dark:text-white")
            }
            style={{ fontSize: 48, lineHeight: 1 }}
          >
            {slo.is_breaching ? "BREACHING" : `${slo.slo_pct.toFixed(2)}%`}
          </div>
          <div className="mt-1 text-xs text-[#898781]">current SLO</div>
        </div>

        <div className="min-w-[220px] flex-1 max-w-sm">
          <div className="flex items-baseline justify-between text-sm">
            <span className="text-[#52514e] dark:text-[#c3c2b7]">Error budget remaining</span>
            <span className="font-medium text-[#0b0b0b] dark:text-white">
              {slo.budget_remaining_pct.toFixed(1)}%
            </span>
          </div>
          <div
            className="mt-2 h-3 w-full overflow-hidden rounded-full"
            style={{ backgroundColor: track }}
            role="meter"
            aria-valuenow={Math.round(pct)}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <div
              className="h-full rounded-full transition-[width]"
              style={{ width: `${pct}%`, backgroundColor: fill }}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
