import { useState } from "react";
import { useDashboardData } from "./hooks/useDashboardData";
import TopBar from "./components/TopBar";
import BurnRateMeters from "./components/BurnRateMeters";
import LatencyChart from "./components/LatencyChart";
import IncidentsTable from "./components/IncidentsTable";
import BadgeEmbed from "./components/BadgeEmbed";

const DEFAULT_PROJECT = import.meta.env.VITE_PROJECT_ID || "example-site";

export default function App() {
  const [projectId, setProjectId] = useState(DEFAULT_PROJECT);
  const { slo, incidents, metrics, error, loading } = useDashboardData(projectId);

  return (
    <div className="min-h-screen bg-[#f9f9f7] dark:bg-[#0d0d0d] text-[#0b0b0b] dark:text-white">
      <div className="mx-auto max-w-5xl px-6 py-8 space-y-6">
        <div className="flex items-center justify-between gap-4">
          <h1 className="text-sm font-semibold text-[#898781] tracking-wide uppercase">
            vigil
          </h1>
          <label className="flex items-center gap-2 text-sm text-[#52514e] dark:text-[#c3c2b7]">
            Project
            <input
              value={projectId}
              onChange={(e) => setProjectId(e.target.value)}
              className="rounded border border-[#c3c2b7] dark:border-[#383835] bg-[#fcfcfb] dark:bg-[#1a1a19] px-2 py-1 text-[#0b0b0b] dark:text-white focus:outline-none focus:ring-2 focus:ring-[#2a78d6]"
            />
          </label>
        </div>

        {error && (
          <div className="rounded border border-[#d03b3b]/30 bg-[#d03b3b]/10 px-4 py-3 text-sm text-[#d03b3b]">
            Failed to load data for "{projectId}": {error}
          </div>
        )}

        {loading && !slo ? (
          <div className="text-sm text-[#898781]">Loading…</div>
        ) : (
          slo && (
            <>
              <TopBar projectId={projectId} slo={slo} />
              <BurnRateMeters slo={slo} />
              <LatencyChart metrics={metrics} />
              <IncidentsTable incidents={incidents} />
              <BadgeEmbed projectId={projectId} />
            </>
          )
        )}
      </div>
    </div>
  );
}
