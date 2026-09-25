"use client";

import { useEffect, useRef, useState } from "react";
import { CompetitorSample } from "@/lib/journey";
import { scanNumber, scanString } from "@/lib/useScanStream";

function faviconLetter(domain: string) {
  return domain.replace(/^www\./, "")[0]?.toUpperCase() ?? "?";
}

function friendlyClass(raw?: string) {
  if (!raw || raw === "Classifying…" || raw === "unknown") return "";
  const lower = raw.toLowerCase();
  if (lower.includes("direct") || lower.includes("d2c")) return "Direct rival";
  if (lower.includes("content") || lower.includes("publisher")) return "Content site";
  if (lower.includes("market")) return "Marketplace";
  if (lower.includes("retail")) return "Retailer";
  return raw.replace(/_/g, " ");
}

function barColor(i: number) {
  const colors = ["bg-[#1a1a18]", "bg-zinc-700", "bg-zinc-600", "bg-zinc-500", "bg-zinc-400"];
  return colors[i] ?? "bg-zinc-300";
}

/** Animated typing text — chars appear one by one */
function TypedText({ text, speed = 28 }: { text: string; speed?: number }) {
  const [shown, setShown] = useState("");
  useEffect(() => {
    setShown("");
    if (!text) return;
    let i = 0;
    const id = window.setInterval(() => {
      i++;
      setShown(text.slice(0, i));
      if (i >= text.length) clearInterval(id);
    }, speed);
    return () => clearInterval(id);
  }, [text, speed]);
  return <>{shown}</>;
}

/** Live check of a real rival homepage. */
function ScanningBrowser({ domain, checking }: { domain: string; checking: string }) {
  const [progress, setProgress] = useState(0);
  const [scanLine, setScanLine] = useState(0);

  useEffect(() => {
    if (!checking) return;
    setProgress(0);
    setScanLine(0);
    const start = Date.now();
    const dur = 4200;
    const id = window.setInterval(() => {
      const p = Math.min(1, (Date.now() - start) / dur);
      setProgress(p);
      if (p >= 1) clearInterval(id);
    }, 60);
    const sl = window.setInterval(() => {
      setScanLine((n) => (n + 1) % 6);
    }, 380);
    return () => {
      clearInterval(id);
      clearInterval(sl);
    };
  }, [checking]);

  if (!checking) return null;

  const lines = [
    `Opening https://${checking}`,
    "Reading title & meta…",
    "Scanning headings…",
    "Collecting page copy…",
    "Checking schema…",
    `Comparing to ${domain}`,
  ];

  return (
    <div className="scan-card-appear overflow-hidden rounded-2xl bg-white shadow-md ring-1 ring-black/[0.07]">
      {/* Browser chrome */}
      <div className="flex items-center gap-2 border-b border-zinc-200 bg-zinc-50 px-3 py-2">
        <span className="h-2 w-2 rounded-full bg-[#ff5f57]" />
        <span className="h-2 w-2 rounded-full bg-[#febc2e]" />
        <span className="h-2 w-2 rounded-full bg-[#28c840]" />
        <div className="ml-1 flex min-w-0 flex-1 items-center gap-2 rounded-full bg-white px-2.5 py-1 text-[10px] text-zinc-500 ring-1 ring-zinc-200">
          <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-amber-400 animate-pulse" />
          <span className="truncate font-mono">https://{checking}</span>
        </div>
        <span className="shrink-0 rounded bg-amber-100 px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wide text-amber-700">Scanning</span>
      </div>
      {/* Progress bar */}
      <div className="h-1 w-full bg-zinc-100">
        <div
          className="h-full rounded-r-full bg-[#4285f4] transition-all duration-300 ease-out"
          style={{ width: `${Math.round(progress * 100)}%` }}
        />
      </div>
      {/* Scan lines */}
      <div className="space-y-2 p-4">
        {lines.slice(0, scanLine + 1).map((line, i) => (
          <div key={i} className="flex items-center gap-2">
            {i < scanLine ? (
              <span className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-emerald-100 text-[9px] font-black text-emerald-600">✓</span>
            ) : (
              <span className="flex h-4 w-4 shrink-0 items-center justify-center">
                <span className="h-2 w-2 rounded-full bg-amber-400 animate-pulse" />
              </span>
            )}
            <span className={`text-xs ${i < scanLine ? "text-zinc-400 line-through" : "font-medium text-zinc-700"}`}>
              {i === scanLine ? <TypedText text={line} speed={22} /> : line}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function CompetitorsPanel({ domain, competitors, values }: { domain: string; competitors: CompetitorSample[]; values: Record<string, unknown> }) {
  const discovered = scanNumber(values, "competitors_discovered") ?? competitors.length;
  const crawled = scanNumber(values, "competitors_crawled") ?? 0;
  const backendChecking = scanString(values, "domain")?.replace(/^www\./, "") ?? "";

  const list = [...competitors]
    .sort((a, b) => (b.appearances ?? 0) - (a.appearances ?? 0))
    .slice(0, 6);

  const maxAppearances = Math.max(1, ...list.map((c) => c.appearances ?? 0));

  const classified = list.filter((c) => c.classification && c.classification !== "Classifying…" && c.classification !== "unknown").length;

  const [visualIdx, setVisualIdx] = useState(0);
  useEffect(() => {
    if (list.length === 0) return;
    const id = window.setInterval(() => {
      setVisualIdx((i) => (i + 1) % list.length);
    }, 4200);
    return () => window.clearInterval(id);
  }, [list.length]);

  const checking =
    backendChecking ||
    list[visualIdx]?.domain.replace(/^www\./, "") ||
    "";

  const statusText = checking
    ? `Scanning ${checking}`
    : discovered === 0
      ? "Finding rivals in search…"
      : crawled > 0
        ? "Rival sites checked"
        : "Checking rival sites…";

  const pct = discovered === 0
    ? 8
    : list.length === 0
      ? 33
      : Math.round(33 + (classified / Math.max(list.length, 1)) * 67);

  const [tick, setTick] = useState(0);
  const tickRef = useRef(0);
  useEffect(() => {
    const id = window.setInterval(() => {
      tickRef.current++;
      setTick(tickRef.current);
    }, 800);
    return () => clearInterval(id);
  }, []);

  const phase3Dots = ["▪", "▪▪", "▪▪▪"][tick % 3];

  return (
    <div className="mx-auto flex h-full min-h-0 w-full max-w-5xl flex-col gap-4 overflow-hidden">

      {/* Top status bar */}
      <div className="shrink-0 overflow-hidden rounded-2xl bg-white shadow-sm ring-1 ring-black/5">
        <div className="flex items-center justify-between gap-3 px-5 py-3">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex gap-1.5">
              <span className="ai-dot" />
              <span className="ai-dot" style={{ animationDelay: "0.15s" }} />
              <span className="ai-dot" style={{ animationDelay: "0.3s" }} />
            </div>
            <p className="min-w-0 truncate text-sm font-medium text-[#1a1a18]">{statusText}</p>
          </div>
          <span className="shrink-0 text-sm font-bold tabular-nums text-zinc-400">{pct}%</span>
        </div>
        <div className="h-1.5 w-full bg-zinc-100">
          <div
            className="h-full rounded-r-full bg-[#1a1a18] transition-all duration-700 ease-out"
            style={{ width: `${pct}%` }}
          />
        </div>
        {/* 3-phase track */}
        <div className="grid grid-cols-3 divide-x divide-zinc-100 border-t border-zinc-100">
          {[
            { label: "Find domains", done: discovered > 0, detail: discovered > 0 ? "Done" : `Scanning${phase3Dots}` },
            { label: "Rank rivals", done: list.length >= 3, detail: list.length >= 3 ? "Done" : `Ranking${phase3Dots}` },
            { label: "Scan sites", done: crawled >= Math.min(list.length, 3) && list.length > 0, detail: crawled >= Math.min(list.length, 3) && list.length > 0 ? "Done" : checking ? `Opening${phase3Dots}` : "Queued" },
          ].map((p) => (
            <div key={p.label} className={`px-4 py-2.5 text-center ${p.done ? "bg-emerald-50/60" : ""}`}>
              <p className={`text-[10px] font-semibold uppercase tracking-wide ${p.done ? "text-emerald-700" : "text-zinc-400"}`}>
                {p.done ? "✓" : "→"} {p.label}
              </p>
              <p className={`mt-0.5 text-[11px] font-medium ${p.done ? "text-emerald-600" : "text-zinc-500"}`}>{p.detail}</p>
            </div>
          ))}
        </div>
      </div>

      {/* Main content: competitor list + active scanner */}
      <div className="flex min-h-0 flex-1 gap-4 overflow-hidden">
        {/* Competitor frequency bars */}
        <div className="flex min-h-0 w-[55%] shrink-0 flex-col overflow-hidden rounded-2xl bg-white shadow-sm ring-1 ring-black/5">
          <div className="shrink-0 border-b border-zinc-100 px-5 py-2.5">
            <p className="text-sm font-bold text-[#1a1a18]">Rivals in search</p>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">
            {list.length === 0 ? (
              <div className="flex h-full flex-col items-center justify-center gap-3 py-10">
                <div className="flex gap-1.5">
                  <span className="ai-dot" /><span className="ai-dot" style={{ animationDelay: "0.15s" }} /><span className="ai-dot" style={{ animationDelay: "0.3s" }} />
                </div>
                <p className="text-sm text-zinc-400">Identifying competitors…</p>
              </div>
            ) : (
              <ul className="divide-y divide-zinc-50 px-5 py-2">
                {list.map((comp, i) => {
                  const compClean = comp.domain.replace(/^www\./, "");
                  const isActive = checking === compClean;
                  const isClassified = Boolean(comp.classification && comp.classification !== "Classifying…" && comp.classification !== "unknown");
                  const barW = Math.max(8, Math.round(((comp.appearances ?? 0) / maxAppearances) * 100));
                  return (
                    <li
                      key={comp.domain}
                      className={`scan-card-appear flex items-center gap-3 py-3 transition-colors duration-300 ${isActive ? "rounded-xl bg-amber-50/60 px-2 -mx-2" : ""}`}
                      style={{ animationDelay: `${i * 60}ms` }}
                    >
                      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-zinc-100 text-sm font-bold text-zinc-600">
                        {faviconLetter(comp.domain)}
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <p className="truncate text-sm font-semibold text-[#1a1a18]">{compClean}</p>
                          {isActive && <span className="shrink-0 rounded-full bg-amber-100 px-1.5 py-0.5 text-[9px] font-bold uppercase text-amber-700">scanning</span>}
                          {isClassified && !isActive && (
                            <span className="shrink-0 rounded-full bg-emerald-100 px-1.5 py-0.5 text-[9px] font-bold uppercase text-emerald-700">
                              {friendlyClass(comp.classification) || "Done"}
                            </span>
                          )}
                        </div>
                        <div className="mt-1.5 flex items-center gap-2">
                          <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-zinc-100">
                            <div
                              className={`h-full rounded-full transition-all duration-1000 ease-out ${barColor(i)}`}
                              style={{ width: `${barW}%` }}
                            />
                          </div>
                          <span className="shrink-0 text-[10px] font-bold tabular-nums text-zinc-400">
                            {comp.bestPosition != null ? `Best #${comp.bestPosition}` : ""}
                          </span>
                        </div>
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </div>

        {/* Right: active scanner or orbit visualization */}
        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto">
          {checking ? (
            <ScanningBrowser domain={domain.replace(/^www\./, "")} checking={checking} />
          ) : (
            /* Orbit visualization when not actively checking */
            <div className="flex flex-1 items-center justify-center overflow-hidden rounded-2xl bg-white/70 ring-1 ring-black/5 p-6">
              <div className="relative flex items-center justify-center">
                <div className="scan-orbit pointer-events-none absolute h-40 w-40 rounded-full border border-zinc-200/80" />
                <div className="scan-orbit-slow pointer-events-none absolute h-64 w-64 rounded-full border border-dashed border-zinc-200/50" />
                <div className="relative z-10 rounded-full bg-[#1a1a18] px-5 py-2.5 text-sm font-semibold text-white shadow-lg">
                  {domain.replace(/^www\./, "")}
                </div>
                {list.slice(0, 4).map((c, i) => {
                  const angle = (i / 4) * 360;
                  const r = 100;
                  const x = Math.cos((angle * Math.PI) / 180) * r;
                  const y = Math.sin((angle * Math.PI) / 180) * r;
                  return (
                    <div
                      key={c.domain}
                      className="scan-card-appear absolute rounded-lg bg-white px-2 py-1 text-[10px] font-semibold text-zinc-600 shadow-sm ring-1 ring-black/5"
                      style={{ transform: `translate(${x}px, ${y}px)`, animationDelay: `${i * 100}ms` }}
                    >
                      {c.domain.replace(/^www\./, "").split(".")[0]}
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
