"use client";

import { useState } from "react";
import { pasteReadyAfter, readableCopy } from "@/lib/readable-copy";

/** Scrollable before/after wording with one-click copy on the suggested text. */
export function FixWordingDiff({
  before,
  after,
  variant = "soft",
}: {
  before?: string | null;
  after: string;
  variant?: "soft" | "brutal";
}) {
  const [copied, setCopied] = useState(false);
  const shownBefore = readableCopy(before, 420);
  const shownAfter = pasteReadyAfter(after);

  async function copyAfter() {
    try {
      await navigator.clipboard.writeText(shownAfter);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      /* ignore */
    }
  }

  if (!shownAfter) return null;

  if (variant === "brutal") {
    return (
      <div className="grid gap-2 sm:grid-cols-2">
        <div className="flex max-h-44 flex-col border border-zinc-300 bg-zinc-50 p-3">
          <span className="b-kicker shrink-0 text-zinc-500">Now</span>
          <div className="fix-scroll mt-1 min-h-0 flex-1 pr-1">
            <p className="whitespace-pre-wrap text-sm text-zinc-600">{shownBefore || "—"}</p>
          </div>
        </div>
        <div className="flex max-h-44 flex-col border-4 border-black bg-[var(--brutal-emerald)] p-3">
          <div className="flex shrink-0 items-center justify-between gap-2">
            <span className="b-kicker">Change to</span>
            <button
              type="button"
              onClick={copyAfter}
              className="border-2 border-black bg-white px-2 py-0.5 text-[10px] font-black uppercase tracking-wide shadow-[2px_2px_0_0_#000] transition hover:translate-x-px hover:translate-y-px hover:shadow-none"
            >
              {copied ? "Copied" : "Copy"}
            </button>
          </div>
          <div className="fix-scroll mt-1 min-h-0 flex-1 pr-1">
            <p className="whitespace-pre-wrap text-sm font-bold">{shownAfter}</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="grid gap-2 sm:grid-cols-2">
      <div className="flex max-h-40 flex-col rounded-xl bg-zinc-50 px-3 py-2.5 ring-1 ring-zinc-100">
        <p className="shrink-0 text-[10px] font-bold uppercase tracking-wide text-zinc-400">Now</p>
        <div className="fix-scroll mt-1 min-h-0 flex-1 pr-1">
          <p className="whitespace-pre-wrap text-sm text-zinc-600">{shownBefore || "—"}</p>
        </div>
      </div>
      <div className="flex max-h-40 flex-col rounded-xl bg-emerald-50 px-3 py-2.5 ring-1 ring-emerald-100">
        <div className="flex shrink-0 items-center justify-between gap-2">
          <p className="text-[10px] font-bold uppercase tracking-wide text-emerald-700">Change to</p>
          <button
            type="button"
            onClick={copyAfter}
            className="rounded-md bg-emerald-700/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-emerald-800 transition hover:bg-emerald-700/20"
          >
            {copied ? "Copied" : "Copy"}
          </button>
        </div>
        <div className="fix-scroll mt-1 min-h-0 flex-1 pr-1">
          <p className="whitespace-pre-wrap text-sm font-medium text-[#1a1a18]">{shownAfter}</p>
        </div>
      </div>
    </div>
  );
}
