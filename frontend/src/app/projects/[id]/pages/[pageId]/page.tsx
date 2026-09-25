import Link from "next/link";
import { Badge, Card, EmptyState, SectionHeader } from "@/components/ui";
import { formatBytes, formatDateTime, formatMs, pathOf } from "@/lib/data";
import { getPageDetail } from "@/lib/server-data";

export const dynamic = "force-dynamic";

/** Everything the crawler measured about one URL. */
export default async function PageDetailPage({
  params,
}: {
  params: Promise<{ id: string; pageId: string }>;
}) {
  const { id, pageId } = await params;
  const detail = await getPageDetail(id, pageId);

  if (!detail) {
    return (
      <div className="b-surface p-8">
        <EmptyState
          title="Page not found"
          hint="This URL is not part of the stored crawl."
          action={
            <Link href={`/projects/${id}/pages`} className="b-btn">
              Back to pages
            </Link>
          }
        />
      </div>
    );
  }

  const { page, issues } = detail;

  const facts: [string, string][] = [
    ["Status", String(page.status ?? "—")],
    ["Indexable", page.indexable ? "yes" : "no (noindex)"],
    ["Canonical", page.canonical ?? "not declared"],
    ["Robots meta", page.robotsMeta ?? "none"],
    ["Content type", page.contentType ?? "—"],
    ["Depth from homepage", String(page.depth)],
    ["Title", page.title ?? "missing"],
    ["Meta description", page.metaDescription ?? "missing"],
    ["H1", page.h1.length > 0 ? page.h1.join(" | ") : "missing"],
    ["H2 count", String(page.h2.length)],
    ["Word count", String(page.wordCount)],
    [
      "Body excerpt",
      page.bodyText
        ? page.bodyText.length > 280
          ? `${page.bodyText.slice(0, 280)}…`
          : page.bodyText
        : "not captured",
    ],
    [
      "FAQ pairs",
      (() => {
        try {
          const faq = JSON.parse(page.faqJson || "[]");
          return Array.isArray(faq) && faq.length > 0 ? `${faq.length} pairs` : "none";
        } catch {
          return "none";
        }
      })(),
    ],
    ["Images", `${page.images} (${page.imagesMissingAlt} missing alt)`],
    ["Internal links", String(page.internalLinks)],
    ["External links", String(page.externalLinks)],
    ["Structured data", page.structuredData.join(", ") || "none"],
    ["Product schema", page.hasProductSchema ? "yes" : "no"],
    ["Price", page.price ?? "not detected"],
    ["Availability", page.availability ?? "not detected"],
    ["Reviews", page.hasReviews ? "yes" : "no"],
    ["Response time", formatMs(page.responseMs)],
    ["HTML size", formatBytes(page.responseBytes)],
    ["Fetched at", formatDateTime(page.fetchedAt)],
  ];

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      <header>
        <Link href={`/projects/${id}/pages`} className="b-label hover:underline">
          ← Back to pages
        </Link>
        <h1 className="b-mono mt-3 break-all text-xl font-black">{pathOf(page.url)}</h1>
        <div className="mt-3 flex flex-wrap gap-2">
          <Badge tone="neutral">{page.pageType}</Badge>
          <Badge
            tone={page.status && page.status >= 400 ? "critical" : page.status === 200 ? "good" : "neutral"}
          >
            {page.status ?? "—"}
          </Badge>
          <Badge tone={issues.length > 0 ? "warning" : "good"}>{issues.length} issues</Badge>
        </div>
        {page.error ? (
          <p className="mt-3 border-4 border-black bg-[var(--brutal-rose)] p-3 text-sm font-bold">
            Fetch error: {page.error}
          </p>
        ) : null}
      </header>

      <section className="grid gap-6 lg:grid-cols-2">
        <div>
          <SectionHeader title="Measured facts" description="Recorded during the crawl." />
          <Card className="flex flex-col">
            {facts.map(([field, value]) => (
              <div
                key={field}
                className="flex items-start justify-between gap-4 py-2 text-sm"
                style={{ borderBottomWidth: 2, borderBottomStyle: "solid", borderColor: "#e4e4e7" }}
              >
                <span className="b-label shrink-0 text-zinc-500">{field}</span>
                <span className="b-mono break-all text-right text-xs font-bold">{value}</span>
              </div>
            ))}
          </Card>
        </div>

        <div>
          <SectionHeader
            title={`Issues on this page (${issues.length})`}
            description="From the latest technical audit."
          />
          {issues.length === 0 ? (
            <Card>
              <p className="text-sm font-bold text-zinc-500">
                No issues were found on this page.
              </p>
            </Card>
          ) : (
            <div className="flex flex-col gap-3">
              {issues.map((issue) => (
                <Card key={issue.id} className="flex flex-col gap-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge tone={issue.severity}>{issue.severity}</Badge>
                    <span className="b-kicker text-zinc-500">{issue.category}</span>
                  </div>
                  <p className="font-black">{issue.title}</p>
                  <p className="text-sm text-zinc-700">{issue.detail}</p>
                  {issue.recommendation ? (
                    <p className="text-sm font-bold">Fix: {issue.recommendation}</p>
                  ) : null}
                  <Link href={`/projects/${id}/fixes`} className="b-btn b-btn-sm self-start">
                    Open fixes
                  </Link>
                </Card>
              ))}
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
