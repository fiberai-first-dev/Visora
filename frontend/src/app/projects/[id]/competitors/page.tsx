import Link from "next/link";
import { Card, EmptyState } from "@/components/ui";
import { type SeoComparisonRow } from "@/lib/data";
import { getCompetitors, getSeoComparison } from "@/lib/server-data";

export const dynamic = "force-dynamic";

function friendlyClass(raw?: string) {
  if (!raw || raw === "unknown") return "Seen in search";
  const lower = raw.toLowerCase();
  if (lower.includes("direct") || lower.includes("d2c")) return "Direct rival";
  if (lower.includes("content") || lower.includes("publisher")) return "Content site";
  if (lower.includes("market")) return "Marketplace";
  if (lower.includes("retail")) return "Retailer";
  if (lower.includes("search")) return "Search";
  return raw.replace(/_/g, " ");
}

function threatScore(c: { appearances: number; sharedQueryCount: number; bestPosition: number | null }) {
  const rankBoost = c.bestPosition != null && c.bestPosition > 0 ? Math.max(0, 20 - c.bestPosition) : 0;
  return c.appearances * 3 + c.sharedQueryCount * 5 + rankBoost;
}

/**
 * Competitor landscape — ranked by real SERP overlap, with a clear you-vs-them strip.
 */
export default async function CompetitorsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  const [competitors, comparison] = await Promise.all([getCompetitors(id), getSeoComparison(id)]);

  const brand = comparison.find((row) => row.isBrand) ?? null;
  const rivals = comparison.filter((row) => !row.isBrand);
  const ranked = [...competitors]
    .sort((a, b) => threatScore(b) - threatScore(a))
    .slice(0, 5);
  const topThreat = ranked[0] ?? null;
  const totalShared = ranked.reduce((s, c) => s + c.sharedQueryCount, 0);
  const avgRivalPages =
    rivals.length > 0
      ? Math.round(rivals.reduce((s, r) => s + r.pagesCrawled, 0) / rivals.length)
      : 0;

  const crawlByDomain = new Map(comparison.map((row) => [row.domain.replace(/^www\./, ""), row]));

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="b-display text-3xl">Competitors</h1>
          <p className="mt-1 text-sm font-bold text-zinc-600">
            Top {ranked.length} rivals from buyer searches
          </p>
        </div>
        <Link href={`/projects/${id}/fixes`} className="b-btn b-btn-primary">
          Close gaps →
        </Link>
      </header>

      {competitors.length === 0 ? (
        <EmptyState
          title="No competitors discovered yet"
          hint="Run an analysis — rivals appear from the searches your buyers make."
          action={
            <Link href={`/projects/${id}/scan`} className="b-btn b-btn-primary">
              Start scan
            </Link>
          }
        />
      ) : (
        <>
          {/* You vs field */}
          <section className="grid gap-3 sm:grid-cols-3">
            <StatTile
              label="Your crawl"
              value={brand ? `${brand.pagesCrawled}` : "—"}
              hint={
                brand && avgRivalPages > 0
                  ? avgRivalPages > brand.pagesCrawled
                    ? `Rivals avg ${avgRivalPages} pages — they cover more`
                    : `Rivals avg ${avgRivalPages} pages — you're in range`
                  : "Pages we opened on your site"
              }
              tone={
                brand && avgRivalPages > brand.pagesCrawled
                  ? "warn"
                  : brand
                    ? "good"
                    : "neutral"
              }
            />
            <StatTile
              label="Top rival"
              value={topThreat?.domain.replace(/^www\./, "") ?? "—"}
              hint={
                topThreat
                  ? `${topThreat.appearances} search hits · ${topThreat.sharedQueryCount} shared queries`
                  : "No overlap yet"
              }
              tone="neutral"
              compactValue
            />
            <StatTile
              label="Query pressure"
              value={`${totalShared}`}
              hint="Times rivals appear on the same queries you care about"
              tone={totalShared > 10 ? "warn" : "neutral"}
            />
          </section>

          {/* Ranked rivals */}
          <section>
            <div className="mb-4 flex items-end justify-between gap-3">
              <div>
                <h2 className="b-display text-xl">Ranked by threat</h2>
                <p className="mt-0.5 text-sm font-bold text-zinc-500">
                  Ranked by search overlap
                </p>
              </div>
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              {ranked.map((competitor, index) => {
                const key = competitor.domain.replace(/^www\./, "");
                const crawl = crawlByDomain.get(key) ?? crawlByDomain.get(competitor.domain);
                return (
                  <RivalCard
                    key={competitor.id}
                    rank={index + 1}
                    projectId={id}
                    competitor={competitor}
                    crawl={crawl}
                    yourPages={brand?.pagesCrawled ?? 0}
                  />
                );
              })}
            </div>
          </section>
        </>
      )}
    </div>
  );
}

function StatTile({
  label,
  value,
  hint,
  tone,
  compactValue,
}: {
  label: string;
  value: string;
  hint: string;
  tone: "good" | "warn" | "neutral";
  compactValue?: boolean;
}) {
  const bg =
    tone === "good"
      ? "bg-[var(--brutal-emerald)]"
      : tone === "warn"
        ? "bg-[var(--brutal-yellow)]"
        : "bg-white";
  return (
    <Card className={`flex flex-col gap-1 ${bg}`}>
      <span className="b-kicker text-zinc-600">{label}</span>
      <p
        className={`font-black text-[#1a1a18] ${
          compactValue ? "truncate text-lg sm:text-xl" : "b-display text-3xl tabular-nums"
        }`}
        title={value}
      >
        {value}
      </p>
      <p className="text-xs font-bold text-zinc-600">{hint}</p>
    </Card>
  );
}

function RivalCard({
  rank,
  projectId,
  competitor,
  crawl,
  yourPages,
}: {
  rank: number;
  projectId: string;
  competitor: {
    id: number;
    domain: string;
    classification: string;
    appearances: number;
    bestPosition: number | null;
    sharedQueryCount: number;
    crawlStatus: string;
    pagesCrawled: number;
  };
  crawl?: SeoComparisonRow;
  yourPages: number;
}) {
  const pages = crawl?.pagesCrawled ?? competitor.pagesCrawled;

  return (
    <Link
      href={`/projects/${projectId}/competitors/${competitor.id}`}
      className="b-card group flex flex-col gap-4 p-4 transition hover:-translate-y-0.5 hover:bg-zinc-50"
    >
      <div className="flex min-w-0 items-start gap-3">
        <span className="flex h-8 w-8 shrink-0 items-center justify-center border-4 border-black bg-black text-sm font-black text-white">
          {rank}
        </span>
        <div className="min-w-0">
          <p className="truncate text-base font-black group-hover:underline">
            {competitor.domain.replace(/^www\./, "")}
          </p>
          <p className="mt-0.5 text-xs font-bold text-zinc-500">
            {friendlyClass(competitor.classification)}
          </p>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-2">
        <Metric
          label="Search hits"
          value={String(competitor.appearances)}
          sub="in your SERPs"
        />
        <Metric
          label="Shared"
          value={String(competitor.sharedQueryCount)}
          sub="of your queries"
        />
        <Metric
          label="Best rank"
          value={competitor.bestPosition != null ? `#${competitor.bestPosition}` : "—"}
          sub="top position"
        />
      </div>

      {pages > 0 ? (
        <div className="flex items-center justify-between gap-2 border-t-4 border-black pt-3 text-xs font-bold">
          <span className="text-zinc-500">Pages checked</span>
          <span className="tabular-nums">
            {pages}
            {yourPages > 0 ? (
              <span className="ml-1 text-zinc-400">
                · you {yourPages}
                {pages > yourPages ? " (they have more)" : pages < yourPages ? " (you have more)" : ""}
              </span>
            ) : null}
          </span>
        </div>
      ) : null}
    </Link>
  );
}

function Metric({ label, value, sub }: { label: string; value: string; sub: string }) {
  return (
    <div className="border-4 border-black bg-zinc-50 px-2 py-2 text-center">
      <p className="b-kicker text-[9px] text-zinc-500">{label}</p>
      <p className="mt-0.5 text-lg font-black tabular-nums text-[#1a1a18]">{value}</p>
      <p className="text-[10px] font-bold text-zinc-400">{sub}</p>
    </div>
  );
}
