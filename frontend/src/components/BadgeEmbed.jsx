import { useState } from "react";
import { badgeURL } from "../api";

export default function BadgeEmbed({ projectId }) {
  const [copied, setCopied] = useState(false);
  const url = badgeURL(projectId);
  const snippet = `![vigil status](${url})`;

  async function copy() {
    await navigator.clipboard.writeText(snippet);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <div className="rounded-lg border border-[#e1e0d9] dark:border-[#2c2c2a] bg-[#fcfcfb] dark:bg-[#1a1a19] p-5">
      <div className="flex items-center justify-between mb-3">
        <div className="text-sm font-medium text-[#0b0b0b] dark:text-white">
          Badge embed
        </div>
        <img src={url} alt="vigil status badge" height={20} />
      </div>
      <div className="flex items-center gap-2">
        <code className="flex-1 truncate rounded bg-[#f0efec] dark:bg-[#0d0d0d] px-3 py-2 text-xs text-[#52514e] dark:text-[#c3c2b7]">
          {snippet}
        </code>
        <button
          onClick={copy}
          className="shrink-0 rounded bg-[#2a78d6] dark:bg-[#3987e5] px-3 py-2 text-xs font-medium text-white hover:opacity-90"
        >
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
    </div>
  );
}
