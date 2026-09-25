import Link from "next/link";
import { Badge, Card, EmptyState, SectionHeader } from "@/components/ui";
import { formatPercent } from "@/lib/data";
import { getGeoVisibility } from "@/lib/server-data";

export const dynamic = "force-dynamic";

const gapLabel: Record<string, string> = {
  missing: "Not named",
  partial: "Partial",
  mentioned: "Named",
};

const gapTone = (gap: string) => {
  if (gap === "missing") return "critical" as const;
  if (gap === "partial") return "warning" as const;
  return "good" as const;
};

/**
 * AI / GEO visibility — buyer questions × ChatGPT / Claude / Gemini / Grok.
 */
export default async function GeoVisibilityPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const geo = await getGeoVisibility(id);

  const prompts = geo.perPrompt;
  const mentioned = prompts.filter((p) => p.mentioned);
  const missing = prompts.filter((p) => p.gap === "missing");
  const modelsNamingYou = geo.modelStats.filter((m) => m.named > 0).length;
  const topRivals = geo.competitorMentions.slice(0, 8);
  const verdict =
    geo.mentionRate != null && geo.mentionRate >= 0.4
      ? "AIs already recommend you for some buyer questions."
      : geo.answersCollected > 0
        ? "AIs answer these questions — but they name someone else."
        : "Run an analysis to see how AIs talk about your category.";

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="b-display text-3xl">AI visibility</h1>
          <p className="mt-1 max-w-2xl text-sm font-bold text-zinc-600">{verdict}</p>
        </div>
        <Link href={`/projects/${id}/fixes`} className="b-btn b-btn-primary">
          Fix AI gaps →
        </Link>
      </header>

      {prompts.length === 0 ? (
        <EmptyState
          title="No AI answers yet"
          hint="Run an analysis — we ask ChatGPT, Claude, Gemini and Grok the questions your buyers ask."
          action={
            <Link href={`/projects/${id}/scan`} className="b-btn b-btn-primary">
              Start scan
            </Link>
          }
        />
      ) : (
        <>
          <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">GEO score</span>
              <span className="b-mono text-3xl font-black tabular-nums">
                {geo.geoScore == null ? "—" : Math.round(geo.geoScore)}
              </span>
              <span className="text-xs font-bold text-zinc-500">
                {geo.geoScore != null && geo.geoScore < 25
                  ? "Low — rarely named"
                  : geo.geoScore != null && geo.geoScore < 55
                    ? "Building presence"
                    : "Strong AI presence"}
              </span>
            </Card>
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">Mention rate</span>
              <span className="b-mono text-3xl font-black tabular-nums">
                {formatPercent(geo.mentionRate)}
              </span>
              <span className="text-xs font-bold text-zinc-500">
                {geo.answersMentioned}/{geo.answersCollected || geo.promptsChecked * 4} answers name you
              </span>
            </Card>
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">Questions covered</span>
              <span className="b-mono text-3xl font-black tabular-nums">
                {mentioned.length}/{prompts.length}
              </span>
              <span className="text-xs font-bold text-zinc-500">
                {missing.length} still missing · {modelsNamingYou}/4 models name you
              </span>
            </Card>
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">Rival share</span>
              <span className="b-mono text-3xl font-black tabular-nums">
                {formatPercent(geo.competitorSov)}
              </span>
              <span className="text-xs font-bold text-zinc-500">
                {topRivals[0]
                  ? `Most named: ${topRivals[0].brand}`
                  : "No rival brands extracted yet"}
              </span>
            </Card>
          </section>

          {geo.modelStats.some((m) => m.checked > 0) ? (
            <section>
              <SectionHeader
                title="Per-model coverage"
                description="How often each assistant names your brand across these questions."
              />
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                {geo.modelStats.map((m) => (
                  <Card key={m.model} className="flex flex-col gap-2 !p-4">
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-sm font-black">{m.model}</span>
                      <span
                        className={`b-mono text-lg font-black tabular-nums ${
                          m.named > 0 ? "text-[var(--brutal-emerald)]" : "text-zinc-400"
                        }`}
                      >
                        {m.named}/{m.checked}
                      </span>
                    </div>
                    <div className="h-2 overflow-hidden border-2 border-black bg-zinc-100">
                      <div
                        className={`h-full ${m.named > 0 ? "bg-[var(--brutal-emerald)]" : "bg-zinc-300"}`}
                        style={{
                          width: `${m.checked > 0 ? Math.round((m.named / m.checked) * 100) : 0}%`,
                        }}
                      />
                    </div>
                    <span className="text-[11px] font-bold text-zinc-500">
                      {formatPercent(m.rate)} of answers
                    </span>
                  </Card>
                ))}
              </div>
            </section>
          ) : null}

          {topRivals.length > 0 ? (
            <section>
              <SectionHeader
                title="Who AIs name instead"
                description="Brands that showed up in the same answers."
              />
              <div className="flex flex-wrap gap-2">
                {topRivals.map((c) => (
                  <span
                    key={c.brand}
                    className="border-4 border-black bg-white px-3 py-2 text-sm font-black"
                  >
                    {c.brand}{" "}
                    <span className="b-mono text-zinc-500">×{c.count}</span>
                  </span>
                ))}
              </div>
            </section>
          ) : null}

          <section className="flex flex-col gap-4">
            <SectionHeader
              title={`${prompts.length} buyer questions`}
              description="Open a question to read what each model actually said."
            />
            {prompts.map((row) => (
              <details
                key={row.promptId || row.text}
                className="group border-4 border-black bg-white open:shadow-[4px_4px_0_#000]"
              >
                <summary className="cursor-pointer list-none px-4 py-3 marker:content-none [&::-webkit-details-marker]:hidden">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <p className="font-black leading-snug text-[#1a1a18]">{row.text}</p>
                      <div className="mt-2 flex flex-wrap items-center gap-2">
                        {row.intent ? (
                          <span className="text-[10px] font-black uppercase tracking-wide text-zinc-400">
                            {row.intent}
                          </span>
                        ) : null}
                        {row.modelsChecked.map((m) => (
                          <span
                            key={m}
                            className={`text-[11px] font-bold ${
                              row.modelsMentioned.includes(m)
                                ? "text-[var(--brutal-emerald)]"
                                : "text-zinc-400"
                            }`}
                          >
                            {m}
                            {row.modelsMentioned.includes(m) ? " ✓" : ""}
                          </span>
                        ))}
                      </div>
                    </div>
                    <div className="flex shrink-0 flex-col items-end gap-1">
                      <Badge tone={gapTone(row.gap)}>{gapLabel[row.gap] ?? row.gap}</Badge>
                      <span className="text-[11px] font-bold text-zinc-500">
                        {row.mentioned
                          ? `${row.mentionCount}/${Math.max(row.modelsChecked.length, 1)} models`
                          : row.bestCompetitor
                            ? `vs ${row.bestCompetitor}`
                            : "not mentioned"}
                      </span>
                      <span className="text-[10px] font-bold uppercase tracking-wide text-zinc-400 group-open:hidden">
                        Show answers ↓
                      </span>
                      <span className="hidden text-[10px] font-bold uppercase tracking-wide text-zinc-400 group-open:inline">
                        Hide ↑
                      </span>
                    </div>
                  </div>
                </summary>

                <div className="border-t-4 border-black px-4 py-4">
                  {row.answers.length === 0 ? (
                    <p className="text-sm font-bold text-zinc-500">No answers stored for this question.</p>
                  ) : (
                    <div className="grid gap-3 lg:grid-cols-2">
                      {row.answers.map((ans) => (
                        <div
                          key={`${row.promptId}-${ans.model}`}
                          className={`border-2 border-black p-3 ${
                            ans.namedYou ? "bg-[var(--brutal-emerald)]/10" : "bg-[#f7f4ef]"
                          }`}
                        >
                          <div className="mb-2 flex items-center justify-between gap-2">
                            <span className="text-xs font-black uppercase tracking-wide">
                              {ans.model}
                            </span>
                            <span
                              className={`text-[10px] font-black uppercase ${
                                ans.namedYou
                                  ? "text-[var(--brutal-emerald)]"
                                  : "text-[var(--brutal-rose)]"
                              }`}
                            >
                              {ans.namedYou ? "names you" : "skips you"}
                            </span>
                          </div>
                          <p className="whitespace-pre-wrap text-[13px] font-bold leading-relaxed text-[#1a1a18]">
                            {ans.text || "—"}
                          </p>
                          {ans.namedBrands.length > 0 ? (
                            <p className="mt-2 text-[11px] font-bold text-zinc-500">
                              Names: {ans.namedBrands.slice(0, 5).join(" · ")}
                            </p>
                          ) : null}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </details>
            ))}
          </section>
        </>
      )}

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm font-bold text-zinc-600">
            {missing.length > 0
              ? `${missing.length} questions still skip your brand — turn them into content and fix briefs.`
              : "Mentions refresh after each analysis."}
          </p>
          <Link href={`/projects/${id}/fixes`} className="b-btn b-btn-sm">
            Turn gaps into fixes →
          </Link>
        </div>
      </Card>
    </div>
  );
}
