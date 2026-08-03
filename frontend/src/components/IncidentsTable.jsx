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

export default function IncidentsTable({ incidents }) {
  return (
    <div className="rounded-lg border border-[#e1e0d9] dark:border-[#2c2c2a] bg-[#fcfcfb] dark:bg-[#1a1a19] p-5">
      <div className="text-sm font-medium text-[#0b0b0b] dark:text-white mb-4">
        Recent incidents
      </div>
      {incidents.length === 0 ? (
        <div className="text-sm text-[#898781] py-6 text-center">
          No incidents recorded.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide text-[#898781] border-b border-[#e1e0d9] dark:border-[#2c2c2a]">
                <th className="py-2 pr-4 font-medium">Fired</th>
                <th className="py-2 pr-4 font-medium">Duration</th>
                <th className="py-2 pr-4 font-medium">Budget consumed</th>
                <th className="py-2 pr-4 font-medium">Issue</th>
              </tr>
            </thead>
            <tbody>
              {incidents.map((inc, i) => (
                <tr
                  key={`${inc.fired_at}-${i}`}
                  className="border-b border-[#e1e0d9] dark:border-[#2c2c2a] last:border-0"
                >
                  <td className="py-2 pr-4 whitespace-nowrap" style={{ fontVariantNumeric: "tabular-nums" }}>
                    {formatTime(inc.fired_at)}
                  </td>
                  <td className="py-2 pr-4">
                    {inc.ongoing ? (
                      <span className="text-[#d03b3b] font-medium">ongoing</span>
                    ) : (
                      formatDuration(inc.breach_duration_sec)
                    )}
                  </td>
                  <td className="py-2 pr-4" style={{ fontVariantNumeric: "tabular-nums" }}>
                    {inc.budget_consumed_pct.toFixed(1)}%
                  </td>
                  <td className="py-2 pr-4">
                    {inc.issue_number ? (
                      GITHUB_REPO ? (
                        <a
                          href={`https://github.com/${GITHUB_REPO}/issues/${inc.issue_number}`}
                          target="_blank"
                          rel="noreferrer"
                          className="text-[#2a78d6] dark:text-[#3987e5] hover:underline"
                        >
                          #{inc.issue_number}
                        </a>
                      ) : (
                        `#${inc.issue_number}`
                      )
                    ) : (
                      "—"
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
