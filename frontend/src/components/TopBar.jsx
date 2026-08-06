export default function TopBar({ projectId }) {
  return (
    <header className="sticky top-0 z-10 border-b border-line bg-white/95 backdrop-blur">
      <div className="mx-auto flex h-14 max-w-[1100px] items-center justify-between gap-4 px-8">
        <nav aria-label="Breadcrumb" className="min-w-0 text-sm">
          <span className="text-ink-secondary">Vigil</span>
          <span className="px-1.5 text-ink-muted">/</span>
          <span className="font-medium text-ink">{projectId}</span>
        </nav>

        <div className="flex shrink-0 items-center gap-4">
          <span className="flex items-center gap-1.5 text-sm text-ink-secondary">
            <span className="live-dot h-2 w-2 rounded-full bg-ok" />
            Live
          </span>
          <a
            href="#badge"
            className="flex items-center gap-1.5 rounded-md border border-line bg-white px-3 py-2 text-sm font-medium text-[#374151] transition-colors hover:border-line-hover hover:bg-subtle"
          >
            {/* lucide "bookmark", inlined — not worth a dependency for one glyph */}
            <svg viewBox="0 0 24 24" aria-hidden="true" className="h-4 w-4">
              <path
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z"
              />
            </svg>
            README badge
          </a>
        </div>
      </div>
    </header>
  );
}
