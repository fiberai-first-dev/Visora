"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { SerpCheckSample } from "@/lib/journey";

// Show up to 12 keyword searches instead of 5
const SHOW_LIMIT = 12;

type Phase = "typing" | "results" | "verdict" | "next";

function useTypewriter(text: string, active: boolean, speed = 28) {
  const [shown, setShown] = useState("");
  useEffect(() => {
    setShown("");
    if (!text || !active) return;
    let i = 0;
    const id = window.setInterval(() => {
      i += 1;
      setShown(text.slice(0, i));
      if (i >= text.length) window.clearInterval(id);
    }, speed);
    return () => window.clearInterval(id);
  }, [text, speed, active]);
  return shown;
}

function displayUrl(row: SerpCheckSample["results"][number]) {
  if (row.url) {
    try {
      const u = new URL(row.url.startsWith("http") ? row.url : `https://${row.url}`);
      const path = u.pathname === "/" ? "" : u.pathname.replace(/\/$/, "");
      return `${u.hostname.replace(/^www\./, "")}${path}`;
    } catch {
      /* fall through */
    }
  }
  return row.domain;
}

function faviconLetter(domain: string) {
  const d = domain.replace(/^www\./, "");
  return (d[0] || "?").toUpperCase();
}

/** Google SERP — only real buyer searches from the pipeline. */
export function GooglePanel({
  domain,
  serpChecks,
  onComplete,
}: {
  domain: string;
  serpChecks: SerpCheckSample[];
  values?: Record<string, unknown>;
  onComplete?: () => void;
}) {
  const orderRef = useRef<string[]>([]);
  const completedRef = useRef(false);

  const playlist = useMemo(() => {
    const byQuery = new Map<string, SerpCheckSample>();
    for (const check of serpChecks) {
      const key = check.query.trim().toLowerCase();
      if (!key) continue;
      byQuery.set(key, check);
      if (orderRef.current.length < SHOW_LIMIT && !orderRef.current.includes(key)) {
        orderRef.current.push(key);
      }
    }

    return orderRef.current
      .map((key) => byQuery.get(key))
      .filter((item): item is SerpCheckSample => Boolean(item));
  }, [serpChecks]);

  const [activeIdx, setActiveIdx] = useState(0);
  const [phase, setPhase] = useState<Phase>("typing");

  const active = playlist[activeIdx] ?? null;
  const query = active?.query ?? "";
  const results = active?.results ?? [];
  const typed = useTypewriter(query, phase === "typing");

  // Start typing on each new query
  useEffect(() => {
    if (!query) return;
    setPhase("typing");
    const typeMs = Math.min(1100, Math.max(550, query.length * 22));
    const id = window.setTimeout(() => setPhase("results"), typeMs);
    return () => window.clearTimeout(id);
  }, [activeIdx, query]);

  const listRef = useRef<HTMLDivElement>(null);

  // Show results and auto-scroll down
  useEffect(() => {
    if (phase !== "results") return;
    const scroller = listRef.current;
    if (!scroller) return;
    scroller.scrollTop = 0;
    const start = window.setTimeout(() => {
      const max = Math.max(0, scroller.scrollHeight - scroller.clientHeight);
      if (max < 40) return;
      const from = scroller.scrollTop;
      const t0 = performance.now();
      const dur = 1400;
      const tick = (now: number) => {
        const t = Math.min(1, (now - t0) / dur);
        const ease = t < 0.5 ? 2 * t * t : -1 + (4 - 2 * t) * t;
        scroller.scrollTop = from + max * ease;
        if (t < 1) requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    }, 320);
    const waitMs = results.length > 0 ? 2200 : 1400;
    const verdictAt = window.setTimeout(() => setPhase("verdict"), waitMs);
    return () => {
      window.clearTimeout(start);
      window.clearTimeout(verdictAt);
    };
  }, [phase, activeIdx, results.length]);

  // Verdict shown → add to history, move to next query
  useEffect(() => {
    if (phase !== "verdict") return;
    const id = window.setTimeout(() => {
      setPhase("next");
    }, 1400);
    return () => window.clearTimeout(id);
  }, [phase, active, query]);

  // Move to next query
  useEffect(() => {
    if (phase !== "next") return;
    const next = activeIdx + 1;
    if (next >= playlist.length) {
      setPhase("typing"); // stay on last
      if (!completedRef.current && playlist.length > 0) {
        completedRef.current = true;
        onComplete?.();
      }
      return;
    }
    setActiveIdx(next);
  }, [phase, activeIdx, playlist.length, onComplete]);

  const found = Boolean(active?.foundInTop50 && (active?.brandPosition ?? 0) > 0);
  const brandPos = active?.brandPosition ?? 0;
  const rivalDomains = useMemo(() => {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const check of playlist) {
      for (const row of check.results) {
        const d = row.domain.replace(/^www\./, "");
        if (!d || row.own || d === domain.replace(/^www\./, "") || seen.has(d)) continue;
        seen.add(d);
        out.push(d);
        if (out.length >= 8) return out;
      }
    }
    return out;
  }, [playlist, domain]);

  return (
    <div className="mx-auto flex h-full min-h-0 w-full max-w-5xl flex-col gap-2">
      {rivalDomains.length > 0 ? (
        <div className="flex shrink-0 items-center gap-2 overflow-hidden">
          <span className="shrink-0 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">
            Found
          </span>
          <div className="flex min-w-0 flex-1 gap-1.5 overflow-hidden">
            {rivalDomains.map((d, i) => (
              <span
                key={d}
                className={`truncate rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                  i === activeIdx % Math.max(rivalDomains.length, 1)
                    ? "bg-amber-100 text-amber-800"
                    : "bg-zinc-100 text-zinc-500"
                }`}
              >
                {d}
              </span>
            ))}
          </div>
        </div>
      ) : null}
      {/* Google SERP card */}
      <div className="scan-card flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl bg-white shadow-md ring-1 ring-black/5">
        {/* Browser chrome */}
        <div className="flex shrink-0 items-center gap-2 border-b border-[#dadce0] bg-[#f8f9fa] px-4 py-2.5">
          <span className="h-2.5 w-2.5 rounded-full bg-[#ff5f57]" />
          <span className="h-2.5 w-2.5 rounded-full bg-[#febc2e]" />
          <span className="h-2.5 w-2.5 rounded-full bg-[#28c840]" />
          <div className="ml-2 flex-1 truncate rounded-full bg-white px-3 py-1.5 text-xs text-[#5f6368] ring-1 ring-[#dadce0]">
            {query
              ? `https://www.google.com/search?q=${encodeURIComponent(query)}`
              : "https://www.google.com/search"}
          </div>
        </div>

        {/* Google UI */}
        <div
          className="shrink-0 border-b border-[#ebebeb] px-5 pb-3 pt-4 sm:px-8"
          style={{ fontFamily: "Arial, Helvetica, sans-serif" }}
        >
          <div className="mb-4 flex items-center gap-6">
            <GoogleWordmark />
            <div className="flex min-h-[48px] flex-1 items-center gap-3 rounded-full border border-[#dfe1e5] bg-white px-5 shadow-[0_1px_6px_rgba(32,33,36,0.08)]">
              <svg width="18" height="18" viewBox="0 0 24 24" className="shrink-0 text-[#9aa0a6]" aria-hidden>
                <path
                  fill="currentColor"
                  d="M15.5 14h-.79l-.28-.27A6.47 6.47 0 0 0 16 9.5 6.5 6.5 0 1 0 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"
                />
              </svg>
              <span className="min-h-[1.25rem] flex-1 text-[16px] text-[#202124]">
                {typed || query}
                {phase === "typing" && typed ? <span className="type-caret" /> : null}
              </span>
              {phase !== "typing" && (
                <svg width="18" height="18" viewBox="0 0 24 24" className="shrink-0 text-[#4285f4]" aria-hidden>
                  <path fill="currentColor" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 14.5v-9l6 4.5-6 4.5z" />
                </svg>
              )}
            </div>
          </div>
          <div className="flex gap-5 border-b border-[#ebebeb] text-[13px]">
            <span className="border-b-[3px] border-[#1a73e8] pb-2 font-medium text-[#1a73e8]">All</span>
            <span className="pb-2 text-[#5f6368]">Images</span>
            <span className="pb-2 text-[#5f6368]">News</span>
            <span className="hidden pb-2 text-[#5f6368] sm:inline">Videos</span>
            <span className="hidden pb-2 text-[#5f6368] sm:inline">Shopping</span>
          </div>
        </div>

        <div className="relative min-h-0 flex-1 overflow-hidden">
          <div
            ref={listRef}
            className="analysis-scroll h-full overflow-y-auto px-5 py-5 sm:px-8"
            style={{ fontFamily: "Arial, Helvetica, sans-serif" }}
          >
            {results.length === 0 || phase === "typing" ? (
              <div className="space-y-7 py-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <div key={i} className="animate-pulse space-y-2.5">
                    <div className="flex items-center gap-2">
                      <div className="h-6 w-6 rounded-full bg-[#f1f3f4]" />
                      <div className="space-y-1">
                        <div className="h-3 w-44 rounded bg-[#f1f3f4]" />
                        <div className="h-2.5 w-32 rounded bg-[#f1f3f4]" />
                      </div>
                    </div>
                    <div className="h-5 w-3/4 rounded bg-[#f1f3f4]" />
                    <div className="h-3.5 w-full rounded bg-[#f1f3f4]" />
                    <div className="h-3.5 w-4/5 rounded bg-[#f1f3f4]" />
                  </div>
                ))}
              </div>
            ) : (
              <ul className="space-y-6 pb-24">
                {results.map((row, index) => {
                  const isBrand =
                    row.own || row.domain.replace(/^www\./, "") === domain.replace(/^www\./, "");
                  const href = displayUrl(row);
                  return (
                    <li key={`${row.domain}-${row.position}-${index}`}>
                      <div className="mb-1 flex items-center gap-2">
                        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[#f1f3f4] text-[11px] font-bold text-[#5f6368]">
                          {faviconLetter(row.domain)}
                        </span>
                        <div className="min-w-0">
                          <div className="truncate text-[13px] text-[#202124]">
                            {row.domain.replace(/^www\./, "")}
                          </div>
                          <div className="truncate text-[12px] text-[#4d5156]">{href}</div>
                        </div>
                        {isBrand ? (
                          <span className="ml-auto shrink-0 rounded bg-[#34a853] px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide text-white">
                            You · #{row.position}
                          </span>
                        ) : null}
                      </div>
                      <a
                        className="block cursor-default text-[20px] leading-snug text-[#1a0dab] hover:underline"
                        tabIndex={-1}
                        onClick={(e) => e.preventDefault()}
                      >
                        {row.title || row.domain}
                      </a>
                      {row.snippet ? (
                        <p className="mt-1 line-clamp-2 text-[14px] leading-[1.58] text-[#4d5156]">
                          {row.snippet}
                        </p>
                      ) : null}
                    </li>
                  );
                })}
              </ul>
            )}
          </div>

          {/* Verdict badge */}
          {phase === "verdict" || phase === "next" ? (
            <div className="pointer-events-none absolute inset-x-0 bottom-0 px-5 pb-4 sm:px-8">
              {found ? (
                <div className="flex items-center gap-2 rounded-lg border border-[#e6f4ea] bg-[#e6f4ea] px-4 py-3 shadow-sm">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[#34a853] text-[10px] font-black text-white">✓</span>
                  <span className="text-xs font-bold text-[#137333]">
                    Found — {domain} at position #{brandPos}
                  </span>
                </div>
              ) : (
                <div className="flex items-center gap-2 rounded-lg border border-[#fce8e6] bg-[#fce8e6] px-4 py-3 shadow-sm">
                  <span className="flex h-5 w-5 items-center justify-center rounded-full bg-[#ea4335] text-[10px] font-black text-white">✕</span>
                  <span className="text-xs font-bold text-[#c5221f]">
                    Not found — {domain} not in top 50 results
                  </span>
                </div>
              )}
            </div>
          ) : null}
        </div>

      </div>
    </div>
  );
}

function GoogleWordmark() {
  return (
    <div className="hidden shrink-0 select-none text-[26px] font-medium tracking-tight sm:block" aria-label="Google">
      <span className="text-[#4285f4]">G</span>
      <span className="text-[#ea4335]">o</span>
      <span className="text-[#fbbc05]">o</span>
      <span className="text-[#4285f4]">g</span>
      <span className="text-[#34a853]">l</span>
      <span className="text-[#ea4335]">e</span>
    </div>
  );
}
