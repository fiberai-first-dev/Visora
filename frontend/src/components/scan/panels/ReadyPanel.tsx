"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { ProjectSummary, getFixes, getRecommendations } from "@/lib/data";
import { pasteReadyAfter } from "@/lib/readable-copy";
import { ScanSnapshot } from "@/lib/useScanStream";

type ActionItem = {
  title: string;
  severity?: string;
  action?: string;
  page_url?: string;
  target_field?: string;
  after?: string;
};

const SHOW = 3;

const FIELD_LABEL: Record<string, string> = {
  title: "Page title",
  meta: "Google description",
  meta_description: "Google description",
  h1: "Main heading",
  body: "Page copy",
  faq: "FAQ",
  schema: "Structured data",
};

function fieldLabel(field?: string) {
  if (!field) return "";
  return FIELD_LABEL[field.toLowerCase()] ?? "";
}

function pathOf(url?: string) {
  if (!url) return "";
  try {
    const u = new URL(url.startsWith("http") ? url : `https://${url}`);
    return u.pathname === "/" ? "Homepage" : u.pathname;
  } catch {
    return url;
  }
}

function cleanTitle(title: string) {
  return title.replace(/^Rank for:\s*/i, "").trim();
}

function dedupe(items: ActionItem[]): ActionItem[] {
  const seen = new Set<string>();
  return items.filter((item) => {
    const key = cleanTitle(item.title || "").toLowerCase();
    if (!key || seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

function severityRank(s?: string) {
  const v = (s || "").toLowerCase();
  return v === "critical" ? 0 : v === "high" ? 1 : v === "warning" ? 2 : 3;
}

function scoreTone(score: number) {
  if (score >= 70) return "text-emerald-700";
  if (score >= 45) return "text-amber-700";
  return "text-[#c45c4a]";
}

export function ReadyPanel({
  projectId,
  summary,
  scan,
}: {
  projectId: string;
  summary: ProjectSummary | null;
  scan: ScanSnapshot;
}) {
  const [pulled, setPulled] = useState<ActionItem[]>([]);
  const [waitedOut, setWaitedOut] = useState(false);
  const [shown, setShown] = useState(false);

  useEffect(() => {
    const id = window.setTimeout(() => setShown(true), 80);
    return () => window.clearTimeout(id);
  }, []);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const [recs, fixes] = await Promise.all([getRecommendations(projectId), getFixes(projectId)]);
        if (cancelled) return;
        const fromFixes = fixes
          .filter((f) => f.status === "pending" || f.status === "ready")
          .map((fix): ActionItem => {
            let after = "";
            let field = fix.fixType;
            let page = fix.pageUrl;
            try {
              const parsed = JSON.parse(fix.content) as Record<string, string>;
              after = parsed.after || parsed.patch || "";
              field = parsed.target_field || field;
              page = parsed.page_url || page;
            } catch {
              after = fix.content;
            }
            return { title: fix.title, severity: "high", page_url: page, target_field: field, after };
          });
        const fromRecs = recs
          .filter((r) => r.status === "open")
          .sort((a, b) => severityRank(a.severity) - severityRank(b.severity))
          .map((r): ActionItem => ({
            title: r.title,
            severity: r.severity,
            action: r.action,
            page_url: r.pageUrl,
            target_field: r.targetField,
            after: r.after,
          }));
        setPulled(dedupe([...fromFixes, ...fromRecs]));
      } catch {
        /* keep stream data */
      }
    };
    void load();
    const id = window.setInterval(() => void load(), 3000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [projectId, scan.finished]);

  useEffect(() => {
    const id = window.setTimeout(() => setWaitedOut(true), 20000);
    return () => window.clearTimeout(id);
  }, []);

  const streamActions = useMemo(
    () => dedupe(Array.isArray(scan.values.top) ? (scan.values.top as ActionItem[]) : []),
    [scan.values.top],
  );

  const actions = useMemo(() => {
    const all = dedupe([...pulled, ...streamActions]);
    // Paste-ready copy first; plain actions only fill empty slots.
    const ready = all.filter((a) => pasteReadyAfter(a.after ?? "").trim());
    const rest = all.filter((a) => !pasteReadyAfter(a.after ?? "").trim());
    return [...ready, ...rest].slice(0, SHOW);
  }, [pulled, streamActions]);

  const writing = !scan.finished && actions.length === 0 && !waitedOut;
  // The summary passed in at page load belongs to the previous scan.
  const siteScore = scan.finished ? summary?.seo.score ?? null : null;
  const aiScore = scan.finished ? summary?.geo.score ?? null : null;

  return (
    <div
      className={`mx-auto flex h-full min-h-0 w-full max-w-2xl flex-col justify-center px-2 py-4 transition-opacity duration-500 ${
        shown ? "opacity-100" : "opacity-0"
      }`}
    >
      <div className="shrink-0 text-center">
        <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-zinc-400">
          {writing ? "Almost there" : "Your report is ready"}
        </p>
        <h2 className="mt-2 font-[family-name:var(--font-display)] text-3xl leading-tight text-[#1a1a18]">
          {writing ? "Writing your fixes…" : "Start with these"}
        </h2>
      </div>

      {siteScore != null || aiScore != null ? (
        <div className="mx-auto mt-6 grid w-full max-w-md shrink-0 grid-cols-2 gap-3">
          {[
            { label: "Site health", value: siteScore },
            { label: "AI visibility", value: aiScore },
          ]
            .filter((s) => s.value != null)
            .map((s) => (
              <div
                key={s.label}
                className="rounded-2xl bg-white px-4 py-3 text-center shadow-sm ring-1 ring-black/5 last:odd:col-span-2"
              >
                <p className={`text-3xl font-black tabular-nums ${scoreTone(Math.round(s.value as number))}`}>
                  {Math.round(s.value as number)}
                  <span className="text-sm font-semibold text-zinc-300">/100</span>
                </p>
                <p className="mt-0.5 text-[11px] font-medium text-zinc-500">{s.label}</p>
              </div>
            ))}
        </div>
      ) : null}

      <ol className="mt-6 min-h-0 space-y-2.5 overflow-y-auto overscroll-contain">
        {writing
          ? [0, 1, 2].map((i) => (
              <li
                key={i}
                className="h-[86px] animate-pulse rounded-2xl bg-white/80 ring-1 ring-black/5"
                style={{ animationDelay: `${i * 120}ms` }}
              />
            ))
          : actions.map((item, i) => {
              const copy = pasteReadyAfter(item.after ?? "").trim();
              const where = [fieldLabel(item.target_field), pathOf(item.page_url)].filter(Boolean).join(" · ");
              return (
                <li
                  key={`${item.title}-${i}`}
                  className="scan-card-appear flex gap-4 rounded-2xl bg-white p-4 shadow-sm ring-1 ring-black/5"
                  style={{ animationDelay: `${i * 120}ms` }}
                >
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[#1a1a18] text-xs font-bold text-white">
                    {i + 1}
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-[15px] font-semibold text-[#1a1a18]">{cleanTitle(item.title)}</p>
                    {where ? <p className="mt-0.5 truncate text-xs text-zinc-400">{where}</p> : null}
                    {copy ? (
                      <p className="mt-2 line-clamp-2 border-l-2 border-emerald-500 pl-3 text-sm leading-snug text-zinc-700">
                        {copy}
                      </p>
                    ) : item.action ? (
                      <p className="mt-2 line-clamp-2 text-sm leading-snug text-zinc-600">{item.action}</p>
                    ) : null}
                  </div>
                </li>
              );
            })}
        {!writing && actions.length === 0 ? (
          <li className="rounded-2xl bg-white px-6 py-8 text-center text-sm text-zinc-500 shadow-sm ring-1 ring-black/5">
            Your fixes are in the workspace.
          </li>
        ) : null}
      </ol>

      <div className="mt-6 flex shrink-0 justify-center">
        <Link
          href={`/projects/${projectId}/fixes`}
          className="inline-flex items-center gap-2 rounded-xl bg-[#1a1a18] px-8 py-3.5 text-sm font-semibold text-white shadow-lg transition hover:-translate-y-0.5 hover:bg-[#a65a3a]"
        >
          See every fix and copy it →
        </Link>
      </div>
    </div>
  );
}
