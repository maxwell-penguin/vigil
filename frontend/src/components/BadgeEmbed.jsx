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
    <div id="badge" className="card scroll-mt-20 p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="text-sm font-medium text-ink">README badge</div>
        <img src={url} alt="vigil status badge" height={20} />
      </div>

      <div className="relative mt-4">
        <button
          onClick={copy}
          className="absolute right-2 top-2 rounded-md border border-white/15 px-2 py-1 text-xs font-medium text-[#d4d4d4] transition-colors hover:bg-white/10 hover:text-white"
        >
          {copied ? "Copied" : "Copy"}
        </button>
        <pre className="overflow-x-auto rounded-lg bg-[#1e1e1e] p-3 pr-20 font-mono text-xs leading-relaxed">
          <code>
            <span className="text-[#d4d4d4]">!</span>
            <span className="text-[#d4d4d4]">[</span>
            <span className="text-[#ce9178]">vigil status</span>
            <span className="text-[#d4d4d4]">]</span>
            <span className="text-[#d4d4d4]">(</span>
            <span className="text-[#569cd6]">{url}</span>
            <span className="text-[#d4d4d4]">)</span>
          </code>
        </pre>
      </div>
    </div>
  );
}
