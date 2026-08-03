import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { useIsDark } from "../hooks/useIsDark";

// First three categorical slots — pre-validated all-pairs safe (CVD + normal
// vision) in both light and dark, per the dataviz palette reference.
const SERIES = {
  p50: { light: "#2a78d6", dark: "#3987e5", label: "p50" },
  p95: { light: "#eb6834", dark: "#d95926", label: "p95" },
  p99: { light: "#1baf7a", dark: "#199e70", label: "p99" },
};

function formatTick(iso) {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export default function LatencyChart({ metrics }) {
  const isDark = useIsDark();
  const ink = isDark ? "#c3c2b7" : "#52514e";
  const grid = isDark ? "#2c2c2a" : "#e1e0d9";
  const surface = isDark ? "#1a1a19" : "#fcfcfb";

  const data = metrics.map((m) => ({
    ...m,
    label: formatTick(m.bucket_start),
  }));

  return (
    <div className="rounded-lg border border-[#e1e0d9] dark:border-[#2c2c2a] bg-[#fcfcfb] dark:bg-[#1a1a19] p-5">
      <div className="text-sm font-medium text-[#0b0b0b] dark:text-white mb-4">
        Latency — last 24h
      </div>
      {data.length === 0 ? (
        <div className="text-sm text-[#898781] py-10 text-center">No data yet</div>
      ) : (
        <ResponsiveContainer width="100%" height={260}>
          <LineChart data={data} margin={{ top: 4, right: 12, bottom: 0, left: 0 }}>
            <CartesianGrid vertical={false} stroke={grid} strokeDasharray="0" />
            <XAxis
              dataKey="label"
              tick={{ fill: ink, fontSize: 11 }}
              axisLine={{ stroke: grid }}
              tickLine={false}
            />
            <YAxis
              tick={{ fill: ink, fontSize: 11 }}
              axisLine={false}
              tickLine={false}
              width={44}
              unit="ms"
            />
            <Tooltip
              contentStyle={{
                backgroundColor: surface,
                border: `1px solid ${grid}`,
                fontSize: 12,
              }}
              labelStyle={{ color: ink }}
            />
            <Legend wrapperStyle={{ fontSize: 12, color: ink }} />
            {Object.entries(SERIES).map(([key, s]) => (
              <Line
                key={key}
                type="monotone"
                dataKey={`${key}_ms`}
                name={s.label}
                stroke={isDark ? s.dark : s.light}
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      )}
    </div>
  );
}
