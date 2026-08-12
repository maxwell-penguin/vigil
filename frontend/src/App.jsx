import { useState } from "react";
import { useDashboardData } from "./hooks/useDashboardData";
import Sidebar from "./components/Sidebar";
import TopBar from "./components/TopBar";
import StatCards, { SLOCards, BurnTrajectoryBanner } from "./components/StatCards";
import BurnRateMeters from "./components/BurnRateMeters";
import LatencyChart from "./components/LatencyChart";
import IncidentsTable from "./components/IncidentsTable";
import BadgeEmbed from "./components/BadgeEmbed";

const DEFAULT_PROJECT = import.meta.env.VITE_PROJECT_ID || "example-site";

export default function App() {
  const [projectId, setProjectId] = useState(DEFAULT_PROJECT);
  const [view, setView] = useState("Overview");
  const { slo, incidents, metrics, error, loading } = useDashboardData(projectId);

  return (
    <div className="flex min-h-screen bg-white text-ink">
      <Sidebar
        projectId={projectId}
        onProjectChange={setProjectId}
        slo={slo}
        view={view}
        onViewChange={setView}
      />

      <div className="min-w-0 flex-1">
        <TopBar projectId={projectId} />

        <main className="mx-auto max-w-[1100px] space-y-6 px-8 py-8">
          {error && (
            <div className="rounded-lg border border-danger/20 bg-danger-soft px-4 py-3 text-sm text-danger">
              Failed to load data for "{projectId}": {error}
            </div>
          )}

          {loading && !slo ? (
            <div className="text-sm text-ink-muted">Loading…</div>
          ) : (
            slo && (
              <>
                <StatCards slo={slo} metrics={metrics} incidents={incidents} />
                <BurnTrajectoryBanner slo={slo} />
                <SLOCards slo={slo} metrics={metrics} />
                <BurnRateMeters slo={slo} />
                <LatencyChart metrics={metrics} />
                <IncidentsTable incidents={incidents} />
                <BadgeEmbed projectId={projectId} />
              </>
            )
          )}
        </main>
      </div>
    </div>
  );
}
