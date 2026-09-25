import Link from "next/link";
import { Badge, Card, DataTable, EmptyState, SectionHeader } from "@/components/ui";
import { getKeywordGaps, getSearchIntents } from "@/lib/server-data";

export const dynamic = "force-dynamic";

const gapTone = (gapType: string) => {
  if (gapType === "missing") return "critical";
  if (gapType === "lagging") return "warning";
  return "good";
};

/** Search visibility: where the brand actually ranks for each discovered query. */
export default async function SearchVisibilityPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  const [intents, gaps] = await Promise.all([getSearchIntents(id), getKeywordGaps(id)]);
  const gapByQuery = new Map(gaps.map((gap) => [gap.query.toLowerCase(), gap]));

  const ranked = intents.filter((intent) => intent.position !== null);
  const topTen = intents.filter((intent) => intent.position !== null && intent.position <= 10);

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="b-display text-3xl">Search visibility</h1>
          <p className="mt-1 text-sm font-bold text-zinc-600">
            {ranked.length} of {intents.length} tracked queries have a live position · {topTen.length}{" "}
            in the top 10
          </p>
        </div>
        <Link href={`/projects/${id}/fixes`} className="b-btn b-btn-primary">
          Fix ranking gaps →
        </Link>
      </header>

      {intents.length === 0 ? (
        <EmptyState
          title="No visibility data yet"
          hint="Run an analysis to see where you appear when buyers search."
          action={
            <Link href={`/projects/${id}/scan`} className="b-btn b-btn-primary">
              Start scan
            </Link>
          }
        />
      ) : (
        <section>
          <SectionHeader
            title={`${intents.length} tracked queries`}
            description="Where you appear for the searches buyers make."
          />
          <DataTable>
            <thead>
              <tr>
                <th>Query</th>
                <th>Intent</th>
                <th>Your position</th>
                <th>Best competitor</th>
                <th>Their position</th>
                <th>Gap</th>
              </tr>
            </thead>
            <tbody>
              {intents.map((intent) => {
                const gap = gapByQuery.get(intent.keyword.toLowerCase());
                return (
                  <tr key={intent.id}>
                    <td className="max-w-[24rem]">
                      <Link
                        href={`/projects/${id}/visibility/${intent.id}`}
                        className="font-bold hover:underline"
                      >
                        {intent.keyword}
                      </Link>
                    </td>
                    <td className="text-xs font-bold uppercase text-zinc-500">{intent.intent}</td>
                    <td className="b-mono">
                      {intent.position === null ? (
                        <span className="font-black text-[var(--brutal-rose)]">not ranking</span>
                      ) : (
                        <span
                          className={
                            intent.position <= 10 ? "font-black text-[var(--brutal-emerald)]" : ""
                          }
                        >
                          #{intent.position}
                        </span>
                      )}
                    </td>
                    <td className="b-mono">{gap?.bestCompetitor ?? "—"}</td>
                    <td className="b-mono">
                      {gap?.bestCompetitorPosition === null || gap === undefined
                        ? "—"
                        : `#${gap.bestCompetitorPosition}`}
                    </td>
                    <td>
                      {gap ? (
                        <Badge tone={gapTone(gap.gapType)}>{gap.gapType}</Badge>
                      ) : (
                        <span className="text-xs font-bold text-zinc-400">—</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </DataTable>
        </section>
      )}

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm font-bold text-zinc-600">
            Rankings refresh after each analysis.
          </p>
          <Link href={`/projects/${id}/fixes`} className="b-btn b-btn-sm">
            Turn gaps into fixes →
          </Link>
        </div>
      </Card>
    </div>
  );
}
