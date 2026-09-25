import { Badge, Card, DataTable, EmptyState, SectionHeader } from "@/components/ui";
import { ApiPage, formatBytes, formatMs, pathOf } from "@/lib/data";
import { getCompetitorDetail } from "@/lib/server-data";
import Link from "next/link";

export const dynamic = "force-dynamic";

/** Competitor detail, derived entirely from stored SERP, GEO and crawl evidence. */
export default async function CompetitorDetailPage({
  params,
}: {
  params: Promise<{ id: string; compId: string }>;
}) {
  const { id, compId } = await params;
  const detail = await getCompetitorDetail(id, compId);

  if (!detail || !detail.competitor) {
    return (
      <div className="b-surface p-8">
        <EmptyState
          title="Competitor not found"
          hint="This competitor does not exist for the project."
          action={
            <Link href={`/projects/${id}/competitors`} className="b-btn">
              Back to competitors
            </Link>
          }
        />
      </div>
    );
  }

  const { competitor, relationshipType, insights, contentGaps, pages } = detail;
  const overlap = insights.keywordOverlap === null ? null : Math.round(insights.keywordOverlap);
  const aiVisibility = insights.aiVisibility === null ? null : Math.round(insights.aiVisibility);

  const metrics = [
    {
      label: "Keyword overlap",
      value: overlap === null ? "—" : `${overlap}%`,
      note: `${insights.sharedQueryCount} shared of ${insights.ownQueryCount} ranking queries`,
    },
    {
      label: "AI visibility",
      value: aiVisibility === null ? "—" : `${aiVisibility}%`,
      note:
        insights.aiPromptsChecked === 0
          ? "no GEO run completed"
          : `${insights.aiMentions} mentions across ${insights.aiPromptsChecked} AI answers`,
    },
    {
      label: "SERP footprint",
      value: String(competitor.appearances),
      note: `across ${insights.trackedQueries} checked queries · best ${
        competitor.bestPosition === null ? "—" : `#${competitor.bestPosition}`
      }`,
    },
  ];

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      <header>
        <Link href={`/projects/${id}/competitors`} className="b-label hover:underline">
          ← Back to competitors
        </Link>
        <h1 className="b-display mt-3 text-3xl">{competitor.domain}</h1>
        <div className="mt-3 flex flex-wrap gap-2">
          <Badge tone="neutral">{competitor.classification.replace(/_/g, " ")}</Badge>
          <Badge tone="neutral">{relationshipType}</Badge>
          <Badge tone={competitor.crawlStatus === "crawled" ? "good" : "neutral"}>
            crawl: {competitor.crawlStatus}
          </Badge>
          <Badge tone="yellow">{insights.trackedPages} pages crawled</Badge>
        </div>
      </header>

      <section className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {metrics.map((metric) => (
          <Card key={metric.label} className="flex flex-col gap-1">
            <span className="b-kicker text-zinc-500">{metric.label}</span>
            <span className="b-mono text-4xl font-black tabular-nums">{metric.value}</span>
            <span className="text-xs font-bold text-zinc-500">{metric.note}</span>
          </Card>
        ))}
      </section>

      <section>
        <SectionHeader
          title="Topics they cover that you don't"
          description="From the real content gap clusters, matched against your own pages."
        />
        {contentGaps.length === 0 ? (
          <Card>
            <p className="text-sm font-bold text-zinc-500">
              No shared gap clusters recorded yet. Gaps appear after the comparison step of a scan.
            </p>
          </Card>
        ) : (
          <div className="flex flex-col gap-4">
            {contentGaps.map((gap) => (
              <Card key={gap.id} className="flex flex-col gap-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h3 className="b-display text-base">{gap.topic}</h3>
                  <div className="flex gap-2">
                    <Badge
                      tone={
                        gap.priority === "critical"
                          ? "critical"
                          : gap.priority === "high"
                            ? "warning"
                            : "notice"
                      }
                    >
                      {gap.priority}
                    </Badge>
                    <Badge tone="neutral">{gap.intent || "unclassified"}</Badge>
                  </div>
                </div>
                {gap.evidence.length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {gap.evidence.slice(0, 8).map((query) => (
                      <span key={query} className="b-badge b-badge-neutral b-mono">
                        {query}
                      </span>
                    ))}
                  </div>
                ) : null}
                <p className="text-sm font-bold">
                  {gap.yourPages.length === 0 ? (
                    <span className="text-[var(--brutal-rose)]">
                      You have no page covering this topic.
                    </span>
                  ) : (
                    <span className="text-zinc-600">
                      You already have {gap.yourPages.length} related page
                      {gap.yourPages.length === 1 ? "" : "s"}: {gap.yourPages[0].url}
                    </span>
                  )}
                </p>
              </Card>
            ))}
          </div>
        )}
      </section>

      <CompetitorPages pages={pages} domain={competitor.domain} />
    </div>
  );
}

function CompetitorPages({ pages, domain }: { pages: ApiPage[]; domain: string }) {
  return (
    <section>
      <SectionHeader
        title={`Pages crawled on ${domain}`}
        description="The real pages this platform fetched from the competitor."
      />
      {pages.length === 0 ? (
        <Card>
          <p className="text-sm font-bold text-zinc-500">This competitor has not been crawled yet.</p>
        </Card>
      ) : (
        <DataTable>
          <thead>
            <tr>
              <th>Path</th>
              <th>Type</th>
              <th>Words</th>
              <th>Response</th>
              <th>HTML</th>
            </tr>
          </thead>
          <tbody>
            {pages.map((page) => (
              <tr key={page.id || page.url}>
                <td className="b-mono max-w-[26rem] truncate" title={page.url}>
                  {pathOf(page.url)}
                </td>
                <td className="text-xs font-bold uppercase">{page.pageType}</td>
                <td className="b-mono">{page.wordCount}</td>
                <td className="b-mono">{formatMs(page.responseMs)}</td>
                <td className="b-mono">{formatBytes(page.responseBytes)}</td>
              </tr>
            ))}
          </tbody>
        </DataTable>
      )}
    </section>
  );
}
