import { useState } from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

const SERIES = [
  { key: "p50_ms", label: "p50", color: "#16a34a" },
  { key: "p95_ms", label: "p95", color: "#d97706" },
  { key: "p99_ms", label: "p99", color: "#dc2626" },
];

// ponytail: tabs are presentation only — the API serves a fixed 24h window,
// so wiring these means a range param on /metrics, which is out of scope here.
const RANGES = ["1h", "6h", "24h", "7d"];

function formatTick(iso) {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export default function LatencyChart({ metrics }) {
  const [range, setRange] = useState("24h");

  const data = metrics.map((m) => ({ ...m, label: formatTick(m.bucket_start) }));
  const last = data.length ? data[data.length - 1] : null;

  return (
    <div className="card p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="text-sm font-medium text-ink">Latency distribution</div>
        <div className="flex items-center gap-0.5 rounded-md bg-subtle p-0.5">
          {RANGES.map((r) => (
            <button
              key={r}
              onClick={() => setRange(r)}
              className={
                "rounded px-2 py-1 text-xs font-medium transition-colors " +
                (r === range
                  ? "bg-white text-ink shadow-sm ring-1 ring-line"
                  : "text-ink-secondary hover:text-ink")
              }
            >
              {r}
            </button>
          ))}
        </div>
      </div>

      {data.length === 0 ? (
        <div className="py-12 text-center text-sm text-ink-muted">No data yet</div>
      ) : (
        <>
          <div className="mt-4">
            <ResponsiveContainer width="100%" height={260}>
              <AreaChart data={data} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
                <defs>
                  {SERIES.map((s) => (
                    <linearGradient key={s.key} id={`fill-${s.key}`} x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor={s.color} stopOpacity={0.1} />
                      <stop offset="100%" stopColor={s.color} stopOpacity={0.01} />
                    </linearGradient>
                  ))}
                </defs>
                <CartesianGrid vertical={false} stroke="#f3f4f6" />
                <XAxis
                  dataKey="label"
                  tick={{ fill: "#9ca3af", fontSize: 11 }}
                  axisLine={{ stroke: "#f3f4f6" }}
                  tickLine={false}
                />
                <YAxis
                  tick={{ fill: "#9ca3af", fontSize: 11 }}
                  axisLine={false}
                  tickLine={false}
                  width={52}
                  unit="ms"
                />
                <Tooltip
                  contentStyle={{
                    backgroundColor: "#ffffff",
                    border: "1px solid #e5e7eb",
                    borderRadius: 8,
                    fontSize: 12,
                    boxShadow: "0 4px 12px rgba(16,24,40,0.08)",
                  }}
                  labelStyle={{ color: "#6b7280", marginBottom: 4 }}
                />
                {SERIES.map((s) => (
                  <Area
                    key={s.key}
                    type="monotone"
                    dataKey={s.key}
                    name={s.label}
                    stroke={s.color}
                    strokeWidth={2}
                    fill={`url(#fill-${s.key})`}
                    dot={false}
                    isAnimationActive={false}
                  />
                ))}
              </AreaChart>
            </ResponsiveContainer>
          </div>

          <div className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-line pt-3">
            {SERIES.map((s) => (
              <div key={s.key} className="flex items-center gap-1.5 text-xs">
                <span
                  className="h-2 w-2 rounded-full"
                  style={{ backgroundColor: s.color }}
                />
                <span className="text-ink-secondary">{s.label}</span>
                <span className="font-mono text-ink">{last ? `${last[s.key]}ms` : "—"}</span>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
