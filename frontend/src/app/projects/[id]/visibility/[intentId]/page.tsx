import Link from "next/link";
import { Badge, Card, DataTable, EmptyState, SectionHeader } from "@/components/ui";
import { getSearchIntentDetail } from "@/lib/server-data";

export const dynamic = "force-dynamic";

/** The real Google result set captured for one query, plus the gap verdict. */
export default async function QueryDetailPage({
  params,
}: {
  params: Promise<{ id: string; intentId: string }>;
}) {
  const { id, intentId } = await params;
  const detail = await getSearchIntentDetail(id, intentId);

  if (!detail) {
    return (
      <div className="b-surface p-8">
        <EmptyState
          title="Query not found"
          hint="This search intent does not exist for the project."
          action={
            <Link href={`/projects/${id}/visibility`} className="b-btn">
              Back to visibility
            </Link>
          }
        />
      </div>
    );
  }

  const { intent, ownPosition, results, keywordGap } = detail;

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      <header>
        <Link href={`/projects/${id}/visibility`} className="b-label hover:underline">
          ← Back to search visibility
        </Link>
        <h1 className="b-display mt-3 text-3xl">{intent.keyword || "Query"}</h1>
        <div className="mt-3 flex flex-wrap gap-2">
          <Badge tone="neutral">{intent.intent}</Badge>
          <Badge tone="neutral">source: {intent.source}</Badge>
          <Badge tone={ownPosition === null ? "critical" : ownPosition <= 10 ? "good" : "warning"}>
            {ownPosition === null ? "not ranking" : `#${ownPosition}`}
          </Badge>
        </div>
      </header>

      {keywordGap ? (
        <Card className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="b-kicker text-zinc-500">Gap verdict</span>
            <Badge tone={keywordGap.gapType === "missing" ? "critical" : "warning"}>
              {keywordGap.gapType}
            </Badge>
          </div>
          <p className="text-sm font-bold">
            {keywordGap.bestCompetitor || "A competitor"} ranks at position{" "}
            {keywordGap.bestCompetitorPosition ?? "—"} for this query.
          </p>
        </Card>
      ) : null}

      <section>
        <SectionHeader
          title={`Search results we captured (${results.length})`}
          description="Exactly what the search API returned for this query."
        />
        {results.length === 0 ? (
          <EmptyState
            title="No results captured"
            hint="This query has not been checked yet, or the search provider returned nothing."
          />
        ) : (
          <DataTable>
            <thead>
              <tr>
                <th>#</th>
                <th>Domain</th>
                <th>Title</th>
                <th>URL</th>
              </tr>
            </thead>
            <tbody>
              {results.map((result, index) => (
                <tr key={`${result.position}-${index}`} className={result.isOwnDomain ? "bg-[var(--brutal-yellow)]" : ""}>
                  <td className="b-mono font-black">{result.position}</td>
                  <td className="font-bold">
                    {result.domain}
                    {result.isOwnDomain ? " (you)" : ""}
                  </td>
                  <td className="max-w-[22rem]">{result.title}</td>
                  <td className="b-mono max-w-[20rem] truncate" title={result.url}>
                    {result.url}
                  </td>
                </tr>
              ))}
            </tbody>
          </DataTable>
        )}
      </section>

      {results.some((result) => result.snippet) ? (
        <section>
          <SectionHeader title="Snippets" description="What the search engine shows for each result." />
          <div className="flex flex-col gap-3">
            {results
              .filter((result) => result.snippet)
              .map((result, index) => (
                <Card key={`${result.position}-snippet-${index}`}>
                  <div className="b-kicker text-zinc-500">
                    {result.position}. {result.domain}
                  </div>
                  <p className="mt-1 text-sm text-zinc-700">{result.snippet}</p>
                </Card>
              ))}
          </div>
        </section>
      ) : null}
    </div>
  );
}
