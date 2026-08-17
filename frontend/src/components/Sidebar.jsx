import { useEffect, useState } from "react";

// SLO Config has no dedicated page yet — points at the badge section, which
// is the closest thing to SLO config the dashboard currently shows.
const VIEWS = [
  { label: "Overview", id: "overview" },
  { label: "Latency", id: "latency" },
  { label: "Incidents", id: "incidents" },
  { label: "SLO Config", id: "badge" },
];

function SectionLabel({ children }) {
  return (
    <div className="px-2 pb-2 pt-6 text-[11px] font-medium uppercase tracking-wider text-ink-muted">
      {children}
    </div>
  );
}

export default function Sidebar({ projectId, onProjectChange, slo }) {
  // No SLO loaded yet reads as neutral rather than falsely healthy.
  const dotColor = !slo ? "bg-ink-muted" : slo.is_breaching ? "bg-danger" : "bg-ok";

  const [inputValue, setInputValue] = useState(projectId);
  useEffect(() => setInputValue(projectId), [projectId]);

  const [activeId, setActiveId] = useState(VIEWS[0].id);

  // The tracked sections only mount once dashboard data has loaded, so this
  // re-runs (and finds the real elements) the moment that flips to true;
  // `slo` itself is a new object on every 30s poll, which would otherwise
  // tear down and recreate the observer on every refresh.
  const hasContent = Boolean(slo);

  useEffect(() => {
    const sections = VIEWS.map((v) => document.getElementById(v.id)).filter(Boolean);
    if (sections.length === 0) return;

    const ratios = new Map(sections.map((el) => [el.id, 0]));

    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => ratios.set(entry.target.id, entry.intersectionRatio));

        let bestId = null;
        let bestRatio = 0;
        ratios.forEach((ratio, id) => {
          if (ratio > bestRatio) {
            bestRatio = ratio;
            bestId = id;
          }
        });
        if (bestId) setActiveId(bestId);
      },
      { threshold: [0, 0.1, 0.25, 0.5, 0.75, 1] }
    );

    sections.forEach((el) => observer.observe(el));
    return () => observer.disconnect();
  }, [hasContent]);

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
        {VIEWS.map(({ label, id }) => {
          const active = id === activeId;
          return (
            <button
              key={id}
              onClick={() =>
                document.getElementById(id)?.scrollIntoView({ behavior: "smooth" })
              }
              className={
                "flex w-full items-center rounded-md border-l-2 px-2 py-1.5 text-left text-sm transition-colors " +
                (active
                  ? "border-accent bg-accent-soft font-medium text-accent"
                  : "border-transparent text-ink-secondary hover:bg-line/50 hover:text-ink")
              }
            >
              {label}
            </button>
          );
        })}
      </nav>
    </aside>
  );
}
