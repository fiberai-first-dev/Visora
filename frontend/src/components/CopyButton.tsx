"use client";

import { useState } from "react";

/** One-click clipboard copy with brief "Copied" feedback. */
export function CopyButton({
  text,
  label = "Copy",
  className = "b-btn b-btn-sm",
}: {
  text: string;
  label?: string;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);

  async function onCopy() {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      /* ignore */
    }
  }

  return (
    <button type="button" onClick={onCopy} className={className} disabled={!text}>
      {copied ? "Copied" : label}
    </button>
  );
}
