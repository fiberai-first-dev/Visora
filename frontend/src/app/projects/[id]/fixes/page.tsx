import Link from "next/link";
import { Badge, Card, EmptyState } from "@/components/ui";
import { FixWordingDiff } from "@/components/FixWordingDiff";
import { RecommendationActions } from "@/components/RecommendationActions";
import { formatDateTime, type ApiFix, type ApiRecommendation } from "@/lib/data";
import { getFixes, getRecommendations } from "@/lib/server-data";

export const dynamic = "force-dynamic";

function parseFixContent(content: string): {
  page_url?: string;
  target_field?: string;
  before?: string;
  after?: string;
  note?: string;
  raw?: string;
} {
  try {
    const parsed = JSON.parse(content) as Record<string, string>;
    if (parsed && typeof parsed === "object" && (parsed.after || parsed.before || parsed.patch)) {
      return {
        ...parsed,
        after: parsed.after || parsed.patch,
      };
    }
  } catch {
    /* plain text */
  }
  return { raw: content };
}

function pathLabel(url?: string) {
  if (!url) return "";
  try {
    const u = new URL(url.startsWith("http") ? url : `https://${url}`);
    return u.pathname || url;
  } catch {
    return url;
  }
}

function dedupeRecs(recs: ApiRecommendation[]): ApiRecommendation[] {
  const seen = new Set<string>();
  const out: ApiRecommendation[] = [];
  for (const rec of recs) {
    const key = rec.title.trim().toLowerCase();
    if (!key || seen.has(key)) continue;
    seen.add(key);
    out.push(rec);
  }
  return out;
}

function severityTone(severity: string): "critical" | "warning" | "notice" | "neutral" {
  const s = severity.toLowerCase();
  if (s === "critical" || s === "high") return "critical";
  if (s === "warning") return "warning";
  if (s === "notice" || s === "low") return "notice";
  return "neutral";
}

/** Single workspace: paste-ready fixes + open recommendations. */
export default async function FixesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const [fixes, recommendations] = await Promise.all([getFixes(id), getRecommendations(id)]);

  const openRecs = dedupeRecs(recommendations.filter((r) => r.status === "open"));
  const storedFixes = fixes.filter((f) => f.status === "pending" || f.status === "ready");
  const pendingFixes =
    storedFixes.length > 0
      ? storedFixes
      : openRecs
          .filter((rec) => Boolean(rec.after))
          .map((rec) => ({
            id: rec.id,
            fixType: rec.targetField || "content",
            pageUrl: rec.pageUrl || "",
            title: rec.title,
            content: JSON.stringify({
              page_url: rec.pageUrl || "",
              target_field: rec.targetField || "body",
              before: rec.before || "",
              after: rec.after,
              note: rec.action,
            }),
            status: "pending",
            createdAt: rec.createdAt,
          }));

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="b-display text-3xl">What to fix</h1>
          <p className="mt-1 text-sm font-bold text-zinc-600">
            {pendingFixes.length} paste-ready
            {openRecs.length > 0 ? ` · ${openRecs.length} open` : ""}
          </p>
        </div>
        <Link href={`/projects/${id}/scan?rerun=1`} className="b-btn b-btn-sm">
          Re-run
        </Link>
      </header>

      <section className="flex flex-col gap-4">
        <h2 className="b-display text-xl">Paste onto your site</h2>

        {pendingFixes.length === 0 ? (
          <EmptyState
            title="No paste-ready fixes yet"
            hint="Re-run analysis and we’ll write the exact copy for each page."
            action={
              openRecs.length > 0 ? (
                <a href="#actions" className="b-btn b-btn-primary">
                  See open actions
                </a>
              ) : (
                <Link href={`/projects/${id}/scan`} className="b-btn b-btn-primary">
                  Start analysis
                </Link>
              )
            }
          />
        ) : (
          <div className="flex flex-col gap-4">
            {pendingFixes.map((fix) => (
              <FixCard key={fix.id} fix={fix} />
            ))}
          </div>
        )}
      </section>

      <section id="actions" className="flex flex-col gap-4">
        <h2 className="b-display text-xl">Open actions</h2>

        {openRecs.length === 0 ? (
          <Card className="py-8 text-center text-sm font-bold text-zinc-500">
            No open recommendations — you&apos;re clear, or run a fresh scan.
          </Card>
        ) : (
          <div className="flex flex-col gap-4">
            {openRecs.map((rec) => (
              <Card key={rec.id} className="flex flex-col gap-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone={severityTone(rec.severity)}>{rec.severity}</Badge>
                  <span className="b-kicker text-zinc-500">{rec.source.replace(/_/g, " ")}</span>
                </div>
                <h3 className="b-display text-base">{rec.title}</h3>
                {rec.detail ? <p className="text-sm text-zinc-700">{rec.detail}</p> : null}
                {(rec.pageUrl || rec.targetField) && (
                  <p className="b-mono text-xs font-bold text-zinc-500">
                    {rec.targetField ? `${rec.targetField} · ` : ""}
                    {rec.pageUrl ? pathLabel(rec.pageUrl) : "site-wide"}
                  </p>
                )}
                {rec.after ? (
                  <FixWordingDiff before={rec.before} after={rec.after} variant="brutal" />
                ) : rec.action ? (
                  <div className="border-4 border-black bg-[var(--brutal-yellow)] p-3">
                    <span className="b-kicker mr-2">Do this</span>
                    <span className="text-sm font-bold">{rec.action}</span>
                  </div>
                ) : null}
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-xs font-bold text-zinc-500">
                    {formatDateTime(rec.createdAt)}
                  </span>
                  <RecommendationActions projectId={id} recommendationId={rec.id} />
                </div>
              </Card>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function FixCard({ fix }: { fix: ApiFix }) {
  const parsed = parseFixContent(fix.content);
  const pageUrl = fix.pageUrl || parsed.page_url || "";
  return (
    <Card className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone="yellow">{parsed.target_field || fix.fixType}</Badge>
          <Badge tone={fix.status === "pending" ? "neutral" : "good"}>{fix.status}</Badge>
        </div>
        <span className="b-mono text-xs font-bold text-zinc-500">{formatDateTime(fix.createdAt)}</span>
      </div>

      <h2 className="b-display text-base">{fix.title}</h2>
      {pageUrl ? <p className="b-mono truncate text-xs font-bold text-zinc-500">{pageUrl}</p> : null}

      {parsed.after ? (
        <FixWordingDiff before={parsed.before} after={parsed.after} variant="brutal" />
      ) : (
        <pre className="b-terminal max-h-72 overflow-auto p-4 text-xs whitespace-pre-wrap">
          {parsed.raw || fix.content}
        </pre>
      )}

      {parsed.note ? (
        <p className="text-sm text-zinc-600">
          <span className="font-bold">Where: </span>
          {parsed.note}
        </p>
      ) : null}
    </Card>
  );
}
