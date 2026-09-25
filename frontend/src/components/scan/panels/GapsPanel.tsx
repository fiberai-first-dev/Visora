"use client";

import { scanNumber, scanRecords } from "@/lib/useScanStream";

export function GapsPanel({ values }: { values: Record<string, unknown> }) {
  const issuesTotal = scanNumber(values, "issues_total") ?? 0;
  const keywordGaps = scanNumber(values, "keyword_gaps") ?? 0;
  const recommendations = scanNumber(values, "recommendations") ?? 0;
  const topIssues = scanRecords(values, "top_issues");
  const severity = scanRecords(values, "issues_by_severity");
  const severityMap: Record<string, number> = {};
  for (const row of severity) {
    for (const [key, val] of Object.entries(row)) severityMap[key] = Number(val) || 0;
  }

  const hasSignal =
    issuesTotal > 0 || keywordGaps > 0 || recommendations > 0 || topIssues.length > 0;

  return (
    <div className="mx-auto w-full max-w-3xl">
      {!hasSignal ? (
        <p className="py-16 text-center text-sm text-zinc-400">
          Gathering gaps from the checks above…
        </p>
      ) : (
        <>
          <div className="grid grid-cols-3 gap-3">
            <Stat label="Site issues" value={issuesTotal} />
            <Stat label="Search gaps" value={keywordGaps} />
            <Stat label="Actions" value={recommendations} />
          </div>

          {(severityMap.critical || severityMap.warning) && (
            <div className="mt-6 space-y-2">
              {severityMap.critical ? (
                <IssueRow tone="critical" label={`${severityMap.critical} critical issues`} />
              ) : null}
              {severityMap.warning ? (
                <IssueRow tone="warning" label={`${severityMap.warning} warnings`} />
              ) : null}
            </div>
          )}

          {topIssues.length > 0 ? (
            <ul className="mt-6 space-y-2">
              {topIssues.slice(0, 5).map((issue, index) => (
                <li
                  key={index}
                  className="flex items-start justify-between gap-3 rounded-xl bg-white px-4 py-3 text-sm shadow-sm ring-1 ring-black/5"
                >
                  <span className="font-medium text-[#1a1a18]">{String(issue.title ?? "")}</span>
                  <span className="shrink-0 font-mono text-xs text-zinc-400">
                    {Number(issue.affected_pages) || 0} pages
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
        </>
      )}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-2xl bg-white px-4 py-5 text-center shadow-sm ring-1 ring-black/5">
      <div className="text-3xl font-semibold tabular-nums text-[#1a1a18]">{value}</div>
      <div className="mt-1 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">
        {label}
      </div>
    </div>
  );
}

function IssueRow({ tone, label }: { tone: "critical" | "warning"; label: string }) {
  return (
    <div
      className={`rounded-xl px-4 py-2.5 text-sm font-medium ${
        tone === "critical"
          ? "bg-red-50 text-red-700 ring-1 ring-red-100"
          : "bg-amber-50 text-amber-800 ring-1 ring-amber-100"
      }`}
    >
      {label}
    </div>
  );
}
