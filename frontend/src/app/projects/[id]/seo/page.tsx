import Link from "next/link";
import { Badge, Card, SectionHeader, Score } from "@/components/ui";
import { formatDateTime, formatMs } from "@/lib/data";
import { getProject, getSeoAudit } from "@/lib/server-data";
import { speedVerdict } from "@/lib/speed";

export const dynamic = "force-dynamic";

/** Technical audit. Every rule and count comes from the stored crawl issues. */
export default async function SeoPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  const project = await getProject(id);
  const audit = await getSeoAudit(id);

  const groups = audit?.issues ?? [];
  const totals = audit?.totals;
  const speed = speedVerdict(audit?.avgResponseMs);
  const speedToneClass =
    speed.tone === "good"
      ? "bg-[var(--brutal-emerald)]"
      : speed.tone === "ok"
        ? "bg-[var(--brutal-yellow)]"
        : speed.tone === "bad"
          ? "bg-[var(--brutal-rose)]"
          : "bg-white";

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="b-display text-3xl">Site health</h1>
          <p className="mt-1 text-sm font-bold text-zinc-600">
            {project?.domain ?? "This site"} · crawl finished{" "}
            {formatDateTime(audit?.crawlFinishedAt)}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Link href={`/projects/${id}/fixes`} className="b-btn b-btn-primary">
            What to fix →
          </Link>
          <Link href={`/projects/${id}/pages`} className="b-btn b-btn-sm">
            Browse pages
          </Link>
        </div>
      </header>

      {groups.length === 0 && audit?.avgResponseMs == null ? (
        <Card>
          <h2 className="b-display text-xl">No audit data yet</h2>
          <p className="mt-1 text-sm font-bold text-zinc-500">
            Run a scan to crawl the site and generate real technical issues. This screen stays empty
            until a crawl stores issues.
          </p>
          <Link href={`/projects/${id}/scan`} className="b-btn b-btn-primary mt-4">
            Start scan
          </Link>
        </Card>
      ) : (
        <>
          <section className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-5">
            <Card className="flex flex-col justify-between gap-2">
              <Score value={audit?.score ?? null} label="SEO score" />
              <p className="text-xs font-bold text-zinc-500">
                Measured {formatDateTime(audit?.computedAt)}
              </p>
            </Card>
            <Card className={`flex flex-col justify-between gap-2 ${speedToneClass}`}>
              <div>
                <span className="b-kicker">Site speed</span>
                <p className="b-mono mt-1 text-3xl font-black tabular-nums">
                  {audit?.avgResponseMs != null ? formatMs(audit.avgResponseMs) : "—"}
                </p>
              </div>
              <p className="text-xs font-black uppercase tracking-wide">{speed.label}</p>
              <p className="text-[10px] font-bold text-zinc-700">
                Avg page response
              </p>
            </Card>
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">Critical rules</span>
              <span className="b-mono text-3xl font-black">{totals?.bySeverity.critical ?? 0}</span>
            </Card>
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">Warning rules</span>
              <span className="b-mono text-3xl font-black">{totals?.bySeverity.warning ?? 0}</span>
            </Card>
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">Notice rules</span>
              <span className="b-mono text-3xl font-black">{totals?.bySeverity.notice ?? 0}</span>
            </Card>
          </section>

          {groups.length === 0 ? (
            <Card>
              <p className="text-sm font-bold text-zinc-500">
                No open issues on this crawl.
              </p>
            </Card>
          ) : (
            <section className="flex flex-col gap-4">
              <SectionHeader
                title={`Issue breakdown (${groups.length} rule types)`}
                description="Ranked by severity and how many pages each rule affects."
              />
              {groups.map((group) => (
                <Card key={group.ruleId} className="flex flex-col gap-3">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone={group.severity}>{group.severity}</Badge>
                      <h3 className="b-display text-base">{group.title}</h3>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="b-badge b-badge-neutral">{group.category}</span>
                      <span className="b-badge b-badge-yellow">{group.affectedPages} pages</span>
                    </div>
                  </div>

                  <p className="text-sm text-zinc-700">{group.detail}</p>

                  {group.recommendation ? (
                    <div className="border-4 border-black bg-[var(--brutal-emerald)] p-3 text-sm font-bold">
                      <span className="b-kicker mr-2">Fix</span>
                      {group.recommendation}
                    </div>
                  ) : null}

                  {group.matchedUrls.length > 0 ? (
                    <details>
                      <summary className="b-label cursor-pointer">
                        Affected pages ({group.matchedUrls.length}
                        {group.affectedPages > group.matchedUrls.length ? "+" : ""})
                      </summary>
                      <ul className="b-mono mt-2 space-y-1 text-xs">
                        {group.matchedUrls.map((url) => (
                          <li key={url} className="truncate">
                            {url}
                          </li>
                        ))}
                      </ul>
                    </details>
                  ) : null}
                </Card>
              ))}
            </section>
          )}
        </>
      )}
    </div>
  );
}
