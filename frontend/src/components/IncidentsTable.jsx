import { useState } from "react";
import IncidentDetail from "./IncidentDetail";

const GITHUB_REPO = import.meta.env.VITE_GITHUB_REPO;

function formatDuration(sec) {
  if (!sec || sec <= 0) return "—";
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${sec}s`;
}

function formatTime(iso) {
  if (!iso) return "—";
  return new Date(iso).toLocaleString();
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center py-10 text-center">
      <div className="flex h-12 w-12 items-center justify-center rounded-full border border-dashed border-line-hover">
        <svg viewBox="0 0 24 24" className="h-5 w-5 text-ink-muted" aria-hidden="true">
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="1.75"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 3l7 3v6c0 4-3 7-7 9-4-2-7-5-7-9V6l7-3zm-2.5 9l1.8 1.8L14.5 10"
          />
        </svg>
      </div>
      <div className="mt-3 text-sm text-ink-muted">No incidents recorded</div>
      <div className="mt-1 text-xs text-ink-muted">
        This project hasn&apos;t breached its SLO in the last 30 days.
      </div>
    </div>
  );
}

// Severe breaches get the red circle; lower-impact ones the yellow triangle.
function Icon({ severe }) {
  if (severe) {
    return <span className="mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full bg-danger" />;
  }
  return (
    <svg viewBox="0 0 12 12" className="mt-1 h-3 w-3 shrink-0 text-warn" aria-hidden="true">
      <path fill="currentColor" d="M6 1l5.5 9.5h-11z" />
    </svg>
  );
}

export default function IncidentsTable({ incidents }) {
  const [expanded, setExpanded] = useState(() => new Set());

  function toggle(key) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  return (
    <div className="card p-5">
      <div className="text-sm font-medium text-ink">Incident history</div>

      {incidents.length === 0 ? (
        <EmptyState />
      ) : (
        <div className="mt-1 divide-y divide-line">
          {incidents.map((inc, i) => {
            const key = `${inc.fired_at}-${i}`;
            const severe = inc.ongoing || inc.budget_consumed_pct >= 50;
            const isOpen = expanded.has(key);
            return (
              <div key={key}>
                <div
                  role="button"
                  tabIndex={0}
                  onClick={() => toggle(key)}
                  onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && toggle(key)}
                  className="flex cursor-pointer items-start justify-between gap-4 py-3 hover:bg-subtle"
                >
                  <div className="flex min-w-0 gap-3">
                    <Icon severe={severe} />
                    <div className="min-w-0">
                      <div className="text-sm text-ink">
                        {inc.ongoing ? "Ongoing error budget breach" : "Error budget breach"}
                      </div>
                      <div className="mt-0.5 text-xs text-ink-secondary">
                        <span className="font-mono">{formatTime(inc.fired_at)}</span>
                        <span className="px-1.5 text-ink-muted">·</span>
                        {inc.budget_consumed_pct.toFixed(1)}% of budget consumed
                      </div>
                    </div>
                  </div>

                  <div className="flex shrink-0 items-center gap-4 text-sm">
                    <span
                      className={
                        "font-mono " + (inc.ongoing ? "font-medium text-danger" : "text-ink-secondary")
                      }
                    >
                      {inc.ongoing ? "Ongoing" : formatDuration(inc.breach_duration_sec)}
                    </span>
                    {inc.issue_number ? (
                      GITHUB_REPO ? (
                        <a
                          href={`https://github.com/${GITHUB_REPO}/issues/${inc.issue_number}`}
                          target="_blank"
                          rel="noreferrer"
                          onClick={(e) => e.stopPropagation()}
                          className="text-accent hover:underline"
                        >
                          #{inc.issue_number}
                        </a>
                      ) : (
                        <span className="text-ink-secondary">#{inc.issue_number}</span>
                      )
                    ) : (
                      <span className="text-ink-muted">No issue</span>
                    )}
                  </div>
                </div>

                {isOpen && <IncidentDetail incident={inc} />}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
