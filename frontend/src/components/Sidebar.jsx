import { useEffect, useState } from "react";

const VIEWS = ["Overview", "Latency", "Incidents", "SLO Config"];

function SectionLabel({ children }) {
  return (
    <div className="px-2 pb-2 pt-6 text-[11px] font-medium uppercase tracking-wider text-ink-muted">
      {children}
    </div>
  );
}

export default function Sidebar({ projectId, onProjectChange, slo, view, onViewChange }) {
  // No SLO loaded yet reads as neutral rather than falsely healthy.
  const dotColor = !slo ? "bg-ink-muted" : slo.is_breaching ? "bg-danger" : "bg-ok";

  const [inputValue, setInputValue] = useState(projectId);
  useEffect(() => setInputValue(projectId), [projectId]);

  return (
    <aside className="sticky top-0 h-screen w-60 shrink-0 overflow-y-auto border-r border-line bg-[#fafafa] px-4 py-5">
      <div className="px-2 text-[15px] font-semibold tracking-tight text-ink">Vigil</div>

      <SectionLabel>Projects</SectionLabel>
      <div className="flex items-center gap-2 rounded-md bg-white px-2 py-1.5 text-sm text-ink ring-1 ring-line">
        <span className={`h-2 w-2 shrink-0 rounded-full ${dotColor}`} />
        <span className="truncate">{projectId}</span>
      </div>
      <label htmlFor="project-select" className="mt-3 block px-2 text-[11px] text-ink-muted">
        Project
      </label>
      <div className="relative mt-1">
        <input
          id="project-select"
          type="text"
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && inputValue.trim()) {
              onProjectChange(inputValue.trim());
            }
          }}
          className="w-full rounded-md border border-line bg-white p-2 text-sm text-ink focus:border-accent focus:outline-none"
        />
      </div>

      <SectionLabel>Views</SectionLabel>
      <nav className="space-y-0.5">
        {VIEWS.map((v) => {
          const active = v === view;
          return (
            <button
              key={v}
              onClick={() => onViewChange(v)}
              className={
                "flex w-full items-center rounded-md border-l-2 px-2 py-1.5 text-left text-sm transition-colors " +
                (active
                  ? "border-accent bg-accent-soft font-medium text-accent"
                  : "border-transparent text-ink-secondary hover:bg-line/50 hover:text-ink")
              }
            >
              {v}
            </button>
          );
        })}
      </nav>
    </aside>
  );
}
