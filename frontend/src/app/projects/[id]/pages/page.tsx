import Link from "next/link";
import { Badge, Card, DataTable, EmptyState } from "@/components/ui";
import { formatMs, issueCountsByUrl, pathOf } from "@/lib/data";
import { getIssues, getPages } from "@/lib/server-data";

export const dynamic = "force-dynamic";

/** Crawled pages from the latest scan. */
export default async function PagesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  const [pages, issues] = await Promise.all([getPages(id), getIssues(id)]);
  const issuesByUrl = issueCountsByUrl(issues);

  const ok = pages.filter((p) => p.status != null && p.status >= 200 && p.status < 400).length;
  const withIssues = pages.filter((p) => (issuesByUrl.get(p.url) ?? 0) > 0).length;
  const avgMs =
    pages.filter((p) => p.responseMs > 0).length > 0
      ? Math.round(
          pages.filter((p) => p.responseMs > 0).reduce((s, p) => s + p.responseMs, 0) /
            pages.filter((p) => p.responseMs > 0).length,
        )
      : null;

  const byType = pages.reduce<Record<string, number>>((acc, page) => {
    acc[page.pageType] = (acc[page.pageType] ?? 0) + 1;
    return acc;
  }, {});

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="b-display text-3xl">Pages</h1>
          <p className="mt-1 text-sm font-bold text-zinc-600">
            {pages.length} URLs from your latest analysis
          </p>
        </div>
        <Link href={`/projects/${id}/seo`} className="b-btn b-btn-sm">
          Site health →
        </Link>
      </header>

      {pages.length === 0 ? (
        <EmptyState
          title="No pages yet"
          hint="Run an analysis to crawl your site."
          action={
            <Link href={`/projects/${id}/scan?rerun=1`} className="b-btn b-btn-primary">
              Start analysis
            </Link>
          }
        />
      ) : (
        <>
          <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">Pages</span>
              <span className="b-mono text-3xl font-black tabular-nums">{pages.length}</span>
            </Card>
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">Healthy</span>
              <span className="b-mono text-3xl font-black tabular-nums">{ok}</span>
            </Card>
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">With issues</span>
              <span className="b-mono text-3xl font-black tabular-nums">{withIssues}</span>
            </Card>
            <Card className="flex flex-col gap-1">
              <span className="b-kicker text-zinc-500">Avg response</span>
              <span className="b-mono text-3xl font-black tabular-nums">
                {avgMs != null ? formatMs(avgMs) : "—"}
              </span>
            </Card>
          </section>

          {Object.keys(byType).length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {Object.entries(byType)
                .sort((a, b) => b[1] - a[1])
                .map(([type, total]) => (
                  <span key={type} className="b-badge b-badge-neutral">
                    {type} · {total}
                  </span>
                ))}
            </div>
          ) : null}

          <section>
            <DataTable>
              <thead>
                <tr>
                  <th>Page</th>
                  <th>Type</th>
                  <th>Status</th>
                  <th>Words</th>
                  <th>Speed</th>
                  <th>Issues</th>
                </tr>
              </thead>
              <tbody>
                {pages.map((page) => {
                  const issueCount = issuesByUrl.get(page.url) ?? 0;
                  const title = (page.title || "").trim();
                  return (
                    <tr key={page.id || page.url}>
                      <td className="max-w-[28rem]">
                        <Link
                          href={`/projects/${id}/pages/${page.id}`}
                          className="block hover:underline"
                          title={page.url}
                        >
                          <span className="block truncate text-sm font-black text-[#1a1a18]">
                            {title || pathOf(page.url)}
                          </span>
                          <span className="b-mono mt-0.5 block truncate text-[11px] font-bold text-zinc-400">
                            {pathOf(page.url)}
                          </span>
                        </Link>
                      </td>
                      <td className="text-xs font-black uppercase text-zinc-500">{page.pageType}</td>
                      <td>
                        <Badge
                          tone={
                            page.status != null && page.status >= 400
                              ? "critical"
                              : page.status === 200
                                ? "good"
                                : "neutral"
                          }
                        >
                          {page.status ?? "—"}
                        </Badge>
                      </td>
                      <td className="b-mono">{page.wordCount || "—"}</td>
                      <td className="b-mono">{formatMs(page.responseMs)}</td>
                      <td>
                        <span
                          className={
                            issueCount > 0
                              ? "font-black text-[var(--brutal-rose)]"
                              : "font-bold text-zinc-400"
                          }
                        >
                          {issueCount}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </DataTable>
          </section>
        </>
      )}
    </div>
  );
}
