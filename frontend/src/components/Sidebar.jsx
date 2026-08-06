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
        <select
          id="project-select"
          value={projectId}
          onChange={(e) => onProjectChange(e.target.value)}
          className="w-full appearance-none rounded-md border border-line bg-white p-2 pr-8 text-sm text-ink focus:border-accent focus:outline-none"
        >
          {/* Only the active project — there's no endpoint that lists projects. */}
          <option value={projectId}>{projectId}</option>
        </select>
        <svg
          viewBox="0 0 24 24"
          aria-hidden="true"
          className="pointer-events-none absolute right-2 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-muted"
        >
          <path
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M6 9l6 6 6-6"
          />
        </svg>
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
