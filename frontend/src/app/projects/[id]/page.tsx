import Link from "next/link";
import { notFound } from "next/navigation";
import { Badge, Card, MetricBar, SectionHeader, severityTone } from "@/components/ui";
import { formatDateTime, formatMs, formatPercent, isRealTimestamp } from "@/lib/data";
import {
  getFixes,
  getIssues,
  getProject,
  getProjectSummary,
  getRecommendations,
  getSeoComparison,
} from "@/lib/server-data";
import { speedVerdict } from "@/lib/speed";

export const dynamic = "force-dynamic";

function dedupeTitles<T extends { title: string }>(items: T[]): T[] {
  const seen = new Set<string>();
  return items.filter((item) => {
    const key = item.title.trim().toLowerCase();
    if (!key || seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export default async function OverviewPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  const project = await getProject(id);
  if (!project) notFound();

  const [summary, issues, comparison, recommendations, fixes] = await Promise.all([
    getProjectSummary(id),
    getIssues(id),
    getSeoComparison(id),
    getRecommendations(id),
    getFixes(id),
  ]);

  const seoScore = summary?.seo.score ?? null;
  const geoScore = summary?.geo.score ?? null;
  const lastCrawlAt = summary?.seo.lastCrawlAt;
  const neverScanned = summary !== null && !isRealTimestamp(lastCrawlAt);
  const avgMs = summary?.seo.avgResponseMs ?? null;
  const speed = speedVerdict(avgMs);
  const speedToneClass =
    speed.tone === "good"
      ? "bg-[var(--brutal-emerald)]"
      : speed.tone === "ok"
        ? "bg-[var(--brutal-yellow)]"
        : speed.tone === "bad"
          ? "bg-[var(--brutal-rose)]"
          : "";

  const openRecs = dedupeTitles(recommendations.filter((r) => r.status === "open")).slice(0, 5);
  const pendingFixes = fixes.filter((f) => f.status === "pending" || f.status === "ready").length;

  const openIssues = issues.filter((issue) => issue.url);
  const severityRank = { critical: 0, warning: 1, notice: 2 } as const;
  const topIssues = openIssues
    .slice()
    .sort((a, b) => severityRank[a.severity] - severityRank[b.severity])
    .slice(0, 3);

  const brand = comparison.find((row) => row.isBrand) ?? null;

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      {summary && summary.activeJobs.length > 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-3 border-4 border-black bg-[var(--brutal-cyan)] px-5 py-4">
          <span className="text-sm font-black uppercase tracking-wide">
            {summary.activeJobs.length} check{summary.activeJobs.length === 1 ? "" : "s"} running
          </span>
          <Link href={`/projects/${id}/scan`} className="b-btn b-btn-sm">
            View live scan
          </Link>
        </div>
      ) : null}

      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="b-display text-3xl md:text-4xl">{project.domain}</h1>
          <p className="mt-1 text-sm font-bold text-zinc-600">
            {project.brand} · {project.category}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Link href={`/projects/${id}/fixes`} className="b-btn b-btn-primary">
            What to fix →
          </Link>
          <Link href={`/projects/${id}/scan?rerun=1`} className="b-btn b-btn-sm">
            Re-run
          </Link>
        </div>
      </header>

      {neverScanned ? (
        <div className="border-4 border-black bg-[var(--brutal-yellow)] p-5">
          <h2 className="b-display text-lg">No scan yet</h2>
          <p className="mt-1 text-sm font-bold">
            Run an analysis to get paste-ready SEO & AI visibility changes for this site.
          </p>
          <Link href={`/projects/${id}/scan`} className="b-btn b-btn-primary mt-4">
            Start scan
          </Link>
        </div>
      ) : null}

      <section className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Card className="flex flex-col gap-1">
          <span className="b-kicker text-zinc-500">SEO health</span>
          <span className="b-mono text-4xl font-black tabular-nums">
            {seoScore === null ? "—" : Math.round(seoScore)}
          </span>
          <span className="text-xs font-bold text-zinc-500">
            {openIssues.length} open site issues
          </span>
        </Card>
        <Card className={`flex flex-col gap-1 ${speedToneClass}`}>
          <span className="b-kicker">Site speed</span>
          <span className="b-mono text-4xl font-black tabular-nums">
            {avgMs != null ? formatMs(avgMs) : "—"}
          </span>
          <span className="text-xs font-black uppercase tracking-wide">{speed.label}</span>
        </Card>
        <Card className="flex flex-col gap-1">
          <span className="b-kicker text-zinc-500">AI visibility</span>
          <span className="b-mono text-4xl font-black tabular-nums">
            {geoScore === null ? "—" : Math.round(geoScore)}
          </span>
          <span className="text-xs font-bold text-zinc-500">
            {geoScore === null
              ? "no AI answers yet"
              : `${summary?.geo.promptsDone ?? 0} answers · ${formatPercent(summary?.geo.mentionRate ?? null)} mentioned`}
          </span>
        </Card>
        <Card className="flex flex-col gap-1 border-4 border-black bg-[var(--brutal-emerald)]">
          <span className="b-kicker">Ready to change</span>
          <span className="b-mono text-4xl font-black tabular-nums">{pendingFixes || openRecs.length}</span>
          <Link href={`/projects/${id}/fixes`} className="mt-1 text-xs font-black uppercase tracking-wide underline">
            {pendingFixes > 0 ? `${pendingFixes} paste-ready fixes` : `${openRecs.length} open actions`} →
          </Link>
        </Card>
      </section>

      <section>
        <SectionHeader
          title="Next steps"
          description="Priority changes from your latest analysis."
        />
        {openRecs.length === 0 && pendingFixes === 0 ? (
          <Card>
            <p className="text-sm font-bold text-zinc-500">
              No open actions. Re-run analysis after you publish changes, or inspect site issues below.
            </p>
          </Card>
        ) : (
          <div className="flex flex-col gap-3">
            {openRecs.map((rec) => (
              <Card key={rec.id} className="flex flex-wrap items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge tone={severityTone(rec.severity)}>{rec.severity}</Badge>
                    <span className="b-kicker text-zinc-500">{rec.source.replace(/_/g, " ")}</span>
                  </div>
                  <h3 className="mt-2 font-black">{rec.title}</h3>
                  {rec.action ? <p className="mt-1 text-sm text-zinc-600">{rec.action}</p> : null}
                  {rec.pageUrl ? (
                    <p className="b-mono mt-2 text-xs font-bold text-zinc-500">{rec.pageUrl}</p>
                  ) : null}
                </div>
                <Link href={`/projects/${id}/fixes`} className="b-btn b-btn-sm shrink-0">
                  Open fix
                </Link>
              </Card>
            ))}
          </div>
        )}
      </section>

      <section className="grid grid-cols-1 gap-8 lg:grid-cols-2">
        <div>
          <SectionHeader title="AI mention rates" description="From ChatGPT, Claude, Gemini & Grok checks." />
          <Card className="flex flex-col gap-6">
            {geoScore === null ? (
              <p className="text-sm font-bold text-zinc-500">Run a scan to measure AI answers.</p>
            ) : (
              <>
                <MetricBar label="Brand mention rate" value={summary?.geo.mentionRate ?? null} />
                <MetricBar label="Citation rate" value={summary?.geo.citationRate ?? null} />
                <MetricBar label="Competitor share of voice" value={summary?.geo.competitorSov ?? null} />
              </>
            )}
          </Card>
        </div>

        <div>
          <SectionHeader title="Site snapshot" description="From the latest crawl." />
          <Card>
            {brand === null ? (
              <p className="text-sm font-bold text-zinc-500">Crawl the site to populate this panel.</p>
            ) : (
              <div className="grid grid-cols-2 gap-4">
                {[
                  { label: "Product pages", value: brand.productPages },
                  { label: "Content pages", value: brand.contentPages },
                  { label: "Avg words", value: Math.round(brand.avgWordCount) || "—" },
                  { label: "Broken pages", value: brand.brokenPages },
                ].map((item) => (
                  <div key={item.label}>
                    <div className="b-kicker text-zinc-500">{item.label}</div>
                    <div className="b-mono text-2xl font-black">{item.value}</div>
                  </div>
                ))}
              </div>
            )}
            <div className="mt-4 flex flex-wrap gap-2">
              <Link href={`/projects/${id}/seo`} className="b-btn b-btn-sm">
                Site health
              </Link>
              <Link href={`/projects/${id}/visibility`} className="b-btn b-btn-sm">
                Google
              </Link>
              <Link href={`/projects/${id}/geo`} className="b-btn b-btn-sm">
                AI answers
              </Link>
              <Link href={`/projects/${id}/competitors`} className="b-btn b-btn-sm">
                Competitors
              </Link>
            </div>
          </Card>
        </div>
      </section>

      {topIssues.length > 0 ? (
        <section>
          <SectionHeader title="Technical notices" description="Lower priority site audit items." />
          <div className="flex flex-col gap-3">
            {topIssues.map((issue) => (
              <Card key={issue.id} className="flex flex-wrap items-start justify-between gap-4">
                <div className="min-w-0">
                  <Badge tone={severityTone(issue.severity)}>{issue.severity}</Badge>
                  <h3 className="mt-2 font-black">{issue.title}</h3>
                  <p className="mt-1 text-sm text-zinc-600">{issue.detail}</p>
                  {issue.path ? (
                    <p className="b-mono mt-2 text-xs font-bold text-zinc-500">{issue.path}</p>
                  ) : null}
                </div>
                <Link href={`/projects/${id}/seo`} className="b-btn b-btn-sm shrink-0">
                  Site health
                </Link>
              </Card>
            ))}
          </div>
        </section>
      ) : null}

      <p className="text-xs font-bold text-zinc-400">
        Last crawl {formatDateTime(lastCrawlAt)} · Updated {formatDateTime(summary?.seo.computedAt)}
      </p>
    </div>
  );
}
