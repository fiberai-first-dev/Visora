// Single API access layer for the frontend.
//
// The Go backend serializes GORM models with their Go field names (PascalCase),
// while hand-built gin.H payloads use camelCase. Rather than depend on one
// convention, every value read here goes through `pick`, so a field rename on
// either side cannot silently turn a real number into a blank cell.

/**
 * API base used by server components. Inside docker-compose this is the internal
 * service name, which the browser cannot resolve.
 */
export const SERVER_API_BASE =
  process.env.API_BASE_URL ||
  process.env.NEXT_PUBLIC_API_URL ||
  "http://localhost:8080/api";

/**
 * API base used by client components. Must be reachable from the browser, so it
 * is always an absolute public URL.
 */
/**
 * Browser calls stay on this Next host so the session cookie is sent.
 * `/go-api` is proxied to the Go API at request time.
 */
export const PUBLIC_API_BASE = "/go-api";

/** Browser-facing API origin (no /api suffix) — used for OAuth and logout. */
export function publicApiOrigin(): string {
  const external =
    process.env.NEXT_PUBLIC_API_URL || "http://localhost:7001/api";
  return external.replace(/\/api\/?$/, "");
}

export type ApiResult<T> = { ok: boolean; status: number; data: T | null };

function requestBase(base?: string) {
  if (base) return base;
  if (typeof window !== "undefined") return PUBLIC_API_BASE;
  return SERVER_API_BASE;
}

/** GET a JSON payload. Never throws: transport failures become `ok: false`. */
export async function apiGet<T = unknown>(
  path: string,
  base?: string,
  headers?: Record<string, string>,
): Promise<ApiResult<T>> {
  try {
    const res = await fetch(`${requestBase(base)}${path}`, {
      cache: "no-store",
      credentials: "include",
      headers: headers ?? {},
    });
    if (!res.ok) return { ok: false, status: res.status, data: null };
    return { ok: true, status: res.status, data: (await res.json()) as T };
  } catch {
    return { ok: false, status: 0, data: null };
  }
}

/** Convenience wrapper returning `null` instead of a result envelope. */
export async function apiGetData<T = unknown>(
  path: string,
  base?: string,
  headers?: Record<string, string>,
): Promise<T | null> {
  const res = await apiGet<T>(path, base, headers);
  return res.ok ? res.data : null;
}

// ── Primitive readers ─────────────────────────────────────────────────────────

/** Read the first present value among `keys` from a payload. */
export function pick<T = unknown>(source: unknown, ...keys: string[]): T | undefined {
  if (!source || typeof source !== "object") return undefined;
  const record = source as Record<string, unknown>;
  for (const key of keys) {
    const value = record[key];
    if (value !== undefined && value !== null) return value as T;
  }
  return undefined;
}

export function pickString(source: unknown, ...keys: string[]): string | null {
  const value = pick(source, ...keys);
  if (value === undefined || value === null) return null;
  const text = String(value).trim();
  return text === "" ? null : text;
}

export function pickNumber(source: unknown, ...keys: string[]): number | null {
  const value = pick(source, ...keys);
  if (value === undefined || value === null || value === "") return null;
  const num = typeof value === "number" ? value : Number(value);
  return Number.isFinite(num) ? num : null;
}

export function pickBool(source: unknown, ...keys: string[]): boolean {
  const value = pick(source, ...keys);
  return value === true || value === "true";
}

export function asArray<T = unknown>(value: unknown): T[] {
  return Array.isArray(value) ? (value as T[]) : [];
}

/** Parse a value that may already be an object or a JSON string (jsonb columns). */
export function parseJson<T>(value: unknown, fallback: T): T {
  if (value === null || value === undefined) return fallback;
  if (typeof value === "object") return value as T;
  if (typeof value !== "string") return fallback;
  const trimmed = value.trim();
  if (trimmed === "") return fallback;
  try {
    return JSON.parse(trimmed) as T;
  } catch {
    return fallback;
  }
}

export function parseStringArray(value: unknown): string[] {
  const parsed = parseJson<unknown>(value, []);
  if (Array.isArray(parsed)) {
    return parsed.filter((item): item is string => typeof item === "string");
  }
  return [];
}

// ── Formatters ────────────────────────────────────────────────────────────────

export function formatMs(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return "—";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
}

export function formatBytes(bytes: number | null | undefined): string {
  if (bytes === null || bytes === undefined || bytes <= 0) return "—";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}

export function formatPercent(fraction: number | null | undefined, digits = 0): string {
  if (fraction === null || fraction === undefined) return "—";
  return `${(fraction * 100).toFixed(digits)}%`;
}

export function hostOf(value: string | null | undefined): string {
  if (!value) return "";
  try {
    return new URL(value.includes("://") ? value : `https://${value}`).hostname.replace(/^www\./, "");
  } catch {
    return value.replace(/^https?:\/\//, "").replace(/^www\./, "").split("/")[0];
  }
}

export function pathOf(value: string | null | undefined): string {
  if (!value) return "";
  try {
    const url = new URL(value.includes("://") ? value : `https://${value}`);
    return `${url.pathname}${url.search}` || "/";
  } catch {
    return value;
  }
}

/** Render an ISO date as a short local date, or `—` when it is missing. */
export function formatDate(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || date.getTime() === 0) return "—";
  return date.toLocaleDateString();
}

/** Render an ISO date+time, or `—` when it is missing. */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || date.getTime() === 0) return "—";
  return date.toLocaleString();
}

/**
 * A date is only "real" when the backend actually stored one. GORM writes the
 * zero time as 0001-01-01, which must not be shown as a measurement date.
 */
export function isRealTimestamp(value: string | null | undefined): boolean {
  if (!value) return false;
  const date = new Date(value);
  return !Number.isNaN(date.getTime()) && date.getUTCFullYear() > 1970;
}

// ── Domain types ──────────────────────────────────────────────────────────────

export type ApiProject = {
  id: string;
  name: string;
  brand: string;
  website: string;
  domain: string;
  category: string;
  country: string;
  status: string;
  createdAt: string | null;
};

export function normalizeProject(raw: unknown): ApiProject | null {
  const fromString = pickString(raw, "id", "publicId", "PublicID", "public_id");
  const fromNumber = pickNumber(raw, "id", "ID");
  const id = fromString || (fromNumber !== null ? String(fromNumber) : null);
  if (!id) return null;
  const website = pickString(raw, "website", "Website") ?? "";
  return {
    id,
    name: pickString(raw, "name", "Name") ?? "Untitled project",
    brand: pickString(raw, "brand", "Brand") ?? "Unknown brand",
    website,
    domain: hostOf(website),
    category: pickString(raw, "category", "Category") ?? "Unknown",
    country: pickString(raw, "country", "Country") ?? "India",
    status: pickString(raw, "status", "Status") ?? "active",
    createdAt: pickString(raw, "createdAt", "CreatedAt"),
  };
}

export type ApiPage = {
  id: number;
  url: string;
  path: string;
  pageType: string;
  status: number | null;
  indexable: boolean;
  title: string | null;
  metaDescription: string | null;
  h1: string[];
  h2: string[];
  canonical: string | null;
  robotsMeta: string | null;
  wordCount: number;
  bodyText: string;
  faqJson: string;
  images: number;
  imagesMissingAlt: number;
  internalLinks: number;
  externalLinks: number;
  structuredData: string[];
  isProductPage: boolean;
  hasProductSchema: boolean;
  hasReviews: boolean;
  price: string | null;
  availability: string | null;
  contentType: string | null;
  depth: number;
  error: string | null;
  responseMs: number;
  responseBytes: number;
  fetchedAt: string | null;
  competitorId: number | null;
};

export function normalizePage(raw: unknown): ApiPage {
  const robotsMeta = pickString(raw, "robotsMeta", "RobotsMeta");
  const url = pickString(raw, "url", "URL") ?? "";
  return {
    id: pickNumber(raw, "id", "ID") ?? 0,
    url,
    path: pathOf(url) || "/",
    pageType: pickString(raw, "pageType", "PageType") ?? "other",
    status: pickNumber(raw, "status", "Status"),
    indexable: !(robotsMeta ?? "").toLowerCase().includes("noindex"),
    title: pickString(raw, "title", "Title"),
    metaDescription: pickString(raw, "metaDescription", "MetaDescription"),
    h1: parseStringArray(pick(raw, "h1", "H1")),
    h2: parseStringArray(pick(raw, "h2", "H2")),
    canonical: pickString(raw, "canonical", "Canonical"),
    robotsMeta,
    wordCount: pickNumber(raw, "wordCount", "WordCount") ?? 0,
    bodyText: pickString(raw, "bodyText", "BodyText") ?? "",
    faqJson: pickString(raw, "faqJson", "FaqJson") ?? "[]",
    images: pickNumber(raw, "images", "Images") ?? 0,
    imagesMissingAlt: pickNumber(raw, "imagesMissingAlt", "ImagesMissingAlt") ?? 0,
    internalLinks: pickNumber(raw, "internalLinks", "InternalLinks") ?? 0,
    externalLinks: pickNumber(raw, "externalLinks", "ExternalLinks") ?? 0,
    structuredData: parseStringArray(pick(raw, "structuredDataTypes", "StructuredDataTypes")),
    isProductPage: pickBool(raw, "isProductPage", "IsProductPage"),
    hasProductSchema: pickBool(raw, "hasProductSchema", "HasProductSchema"),
    hasReviews: pickBool(raw, "hasReviews", "HasReviews"),
    price: pickString(raw, "price", "Price"),
    availability: pickString(raw, "availability", "Availability"),
    contentType: pickString(raw, "contentType", "ContentType"),
    depth: pickNumber(raw, "depth", "Depth") ?? 0,
    error: pickString(raw, "error", "Error"),
    responseMs: pickNumber(raw, "responseMs", "ResponseMs") ?? 0,
    responseBytes: pickNumber(raw, "responseBytes", "ResponseBytes") ?? 0,
    fetchedAt: pickString(raw, "fetchedAt", "FetchedAt"),
    competitorId: pickNumber(raw, "competitorId", "CompetitorID"),
  };
}

export type ApiIssue = {
  id: number;
  url: string | null;
  path: string | null;
  ruleId: string;
  severity: "critical" | "warning" | "notice";
  category: string;
  title: string;
  detail: string;
  recommendation: string;
  crawlRunId: number;
  createdAt: string | null;
};

function toSeverity(value: string | null): ApiIssue["severity"] {
  const normalized = (value ?? "notice").toLowerCase();
  if (normalized === "critical" || normalized === "high") return "critical";
  if (normalized === "warning" || normalized === "medium") return "warning";
  return "notice";
}

export function normalizeIssue(raw: unknown): ApiIssue {
  const url = pickString(raw, "url", "URL");
  return {
    id: pickNumber(raw, "id", "ID") ?? 0,
    url,
    path: url ? pathOf(url) : null,
    ruleId: pickString(raw, "ruleId", "RuleID") ?? "unknown",
    severity: toSeverity(pickString(raw, "severity", "Severity")),
    category: pickString(raw, "category", "Category") ?? "other",
    title: pickString(raw, "title", "Title") ?? "Untitled issue",
    detail: pickString(raw, "detail", "Detail") ?? "",
    recommendation: pickString(raw, "recommendation", "Recommendation") ?? "",
    crawlRunId: pickNumber(raw, "crawlRunId", "CrawlRunID") ?? 0,
    createdAt: pickString(raw, "createdAt", "CreatedAt"),
  };
}

export type ApiIssueGroup = {
  ruleId: string;
  title: string;
  severity: "critical" | "warning" | "notice";
  category: string;
  detail: string;
  recommendation: string;
  affectedPages: number;
  exampleUrl: string | null;
  matchedUrls: string[];
};

export function normalizeIssueGroup(raw: unknown): ApiIssueGroup {
  return {
    ruleId: pickString(raw, "rule_id", "RuleID") ?? "unknown",
    title: pickString(raw, "title", "Title") ?? "Untitled issue",
    severity: toSeverity(pickString(raw, "severity", "Severity")),
    category: pickString(raw, "category", "Category") ?? "other",
    detail: pickString(raw, "detail", "Detail") ?? "",
    recommendation: pickString(raw, "recommendation", "Recommendation") ?? "",
    affectedPages: pickNumber(raw, "affected_pages", "AffectedPages") ?? 0,
    exampleUrl: pickString(raw, "example_url", "ExampleURL"),
    matchedUrls: parseStringArray(pick(raw, "matched_urls", "MatchedURLs")),
  };
}

export type ApiSearchIntent = {
  id: number;
  keyword: string;
  intent: string;
  source: string;
  priority: number;
  status: string;
  position: number | null;
};

export function normalizeSearchIntent(raw: unknown): ApiSearchIntent {
  return {
    id: pickNumber(raw, "id", "ID") ?? 0,
    keyword: pickString(raw, "keyword", "Keyword") ?? "",
    intent: pickString(raw, "intent", "Intent") ?? "informational",
    source: pickString(raw, "source", "Source") ?? "unknown",
    priority: pickNumber(raw, "priority", "Priority") ?? 0,
    status: pickString(raw, "status", "Status") ?? "opportunity",
    position: pickNumber(raw, "position", "Position"),
  };
}

export type ApiSerpResult = {
  position: number;
  domain: string;
  title: string;
  url: string;
  snippet: string;
  isOwnDomain: boolean;
};

export function normalizeSerpResult(raw: unknown): ApiSerpResult {
  return {
    position: pickNumber(raw, "position", "Position") ?? 0,
    domain: pickString(raw, "domain", "Domain") ?? "",
    title: pickString(raw, "pageTitle", "PageTitle") ?? "",
    url: pickString(raw, "pageUrl", "PageURL") ?? "",
    snippet: pickString(raw, "pageSnippet", "PageSnippet") ?? "",
    isOwnDomain: pickBool(raw, "isOwnDomain", "IsOwnDomain"),
  };
}

export type ApiKeywordGap = {
  id: number;
  query: string;
  brandPosition: number | null;
  bestCompetitor: string;
  bestCompetitorPosition: number | null;
  gapType: "missing" | "lagging" | string;
};

export function normalizeKeywordGap(raw: unknown): ApiKeywordGap {
  return {
    id: pickNumber(raw, "id", "ID") ?? 0,
    query: pickString(raw, "query", "Query") ?? "",
    brandPosition: pickNumber(raw, "brandPosition", "BrandPosition"),
    bestCompetitor: pickString(raw, "bestCompetitor", "BestCompetitor") ?? "",
    bestCompetitorPosition: pickNumber(raw, "bestCompetitorPosition", "BestCompetitorPosition"),
    gapType: (pickString(raw, "gapType", "GapType") ?? "missing").toLowerCase(),
  };
}

export type ApiContentGap = {
  id: number;
  topic: string;
  intent: string;
  competitorDomains: string[];
  evidenceQueries: string[];
  existingPages: { url: string; title: string }[];
  priority: string;
  status: string;
};

export function normalizeContentGap(raw: unknown): ApiContentGap {
  return {
    id: pickNumber(raw, "id", "ID") ?? 0,
    topic: pickString(raw, "topic", "Topic") ?? "",
    intent: pickString(raw, "intent", "Intent") ?? "",
    competitorDomains: parseStringArray(pick(raw, "competitorDomains", "CompetitorDomains")),
    evidenceQueries: parseStringArray(pick(raw, "evidenceQueries", "EvidenceQueries")),
    existingPages: parseJson<{ url: string; title: string }[]>(
      pick(raw, "existingRelatedPages", "ExistingRelatedPages"),
      [],
    ),
    priority: (pickString(raw, "priority", "Priority") ?? "medium").toLowerCase(),
    status: pickString(raw, "status", "Status") ?? "open",
  };
}

export type ApiCompetitor = {
  id: number;
  domain: string;
  brandName: string;
  classification: string;
  relationshipType: string;
  appearances: number;
  bestPosition: number | null;
  sharedQueryCount: number;
  crawlStatus: string;
  pagesCrawled: number;
};

export function normalizeCompetitor(raw: unknown): ApiCompetitor {
  return {
    id: pickNumber(raw, "id", "ID") ?? 0,
    domain: pickString(raw, "domain", "Domain") ?? "",
    brandName: pickString(raw, "brand_name", "BrandName") ?? "",
    classification: pickString(raw, "classification", "Classification") ?? "unknown",
    relationshipType: pickString(raw, "type", "Type") ?? "unknown",
    appearances: pickNumber(raw, "appearances", "Appearances") ?? 0,
    bestPosition: pickNumber(raw, "best_position", "BestPosition"),
    sharedQueryCount: pickNumber(raw, "shared_query_count", "SharedQueryCount") ?? 0,
    crawlStatus: pickString(raw, "crawl_status", "CrawlStatus") ?? "pending",
    pagesCrawled: pickNumber(raw, "pages_crawled", "PagesCrawled") ?? 0,
  };
}

export type ApiCompetitorInsights = {
  keywordOverlap: number | null;
  sharedQueryCount: number;
  ownQueryCount: number;
  aiVisibility: number | null;
  aiMentions: number;
  aiPromptsChecked: number;
  topSharedTopics: string[];
  trackedPages: number;
  trackedQueries: number;
  crawlStatus: string;
};

export type ApiRecommendation = {
  id: number;
  source: string;
  severity: "critical" | "warning" | "notice";
  title: string;
  detail: string;
  action: string;
  status: string;
  createdAt: string | null;
  pageUrl?: string;
  targetField?: string;
  before?: string;
  after?: string;
};

export function normalizeRecommendation(raw: unknown): ApiRecommendation {
  const dataRaw = pick(raw, "data", "Data");
  let data: Record<string, unknown> = {};
  if (typeof dataRaw === "string" && dataRaw) {
    try {
      data = JSON.parse(dataRaw) as Record<string, unknown>;
    } catch {
      data = {};
    }
  } else if (dataRaw && typeof dataRaw === "object") {
    data = dataRaw as Record<string, unknown>;
  }

  return {
    id: pickNumber(raw, "id", "ID") ?? 0,
    source: pickString(raw, "source", "Source") ?? "unknown",
    severity: toSeverity(pickString(raw, "severity", "Severity")),
    title: pickString(raw, "title", "Title") ?? "Untitled recommendation",
    detail: pickString(raw, "detail", "Detail") ?? "",
    action: pickString(raw, "action", "Action") ?? "",
    status: pickString(raw, "status", "Status") ?? "open",
    createdAt: pickString(raw, "createdAt", "CreatedAt"),
    pageUrl: pickString(data, "page_url") ?? undefined,
    targetField: pickString(data, "target_field") ?? undefined,
    before: pickString(data, "before") ?? undefined,
    after: pickString(data, "after") ?? undefined,
  };
}

export type ApiFix = {
  id: number;
  fixType: string;
  pageUrl: string;
  title: string;
  content: string;
  status: string;
  createdAt: string | null;
};

export function normalizeFix(raw: unknown): ApiFix {
  return {
    id: pickNumber(raw, "id", "ID") ?? 0,
    fixType: pickString(raw, "fixType", "FixType") ?? "content",
    pageUrl: pickString(raw, "pageUrl", "PageURL") ?? "",
    title: pickString(raw, "title", "Title") ?? "Untitled fix",
    content: pickString(raw, "content", "Content") ?? "",
    status: pickString(raw, "status", "Status") ?? "pending",
    createdAt: pickString(raw, "createdAt", "CreatedAt"),
  };
}

export type ApiMonitorEvent = {
  id: number;
  eventType: string;
  title: string;
  detail: Record<string, unknown>;
  severity: string;
  createdAt: string | null;
};

export function normalizeMonitorEvent(raw: unknown): ApiMonitorEvent {
  return {
    id: pickNumber(raw, "id", "ID") ?? 0,
    eventType: pickString(raw, "eventType", "EventType") ?? "change",
    title: pickString(raw, "title", "Title") ?? "Change detected",
    detail: parseJson<Record<string, unknown>>(pick(raw, "detail", "Detail"), {}),
    severity: (pickString(raw, "severity", "Severity") ?? "notice").toLowerCase(),
    createdAt: pickString(raw, "createdAt", "CreatedAt"),
  };
}

export type ApiMetricsHistory = {
  id: number;
  seoScore: number | null;
  keywordsRanking: number;
  topTenCount: number;
  competitorCount: number;
  computedAt: string | null;
};

export function normalizeMetricsHistory(raw: unknown): ApiMetricsHistory {
  return {
    id: pickNumber(raw, "id", "ID") ?? 0,
    seoScore: pickNumber(raw, "seoScore", "SeoScore"),
    keywordsRanking: pickNumber(raw, "keywordsRanking", "KeywordsRanking") ?? 0,
    topTenCount: pickNumber(raw, "topTenCount", "TopTenCount") ?? 0,
    competitorCount: pickNumber(raw, "competitorCount", "CompetitorCount") ?? 0,
    computedAt: pickString(raw, "computedAt", "ComputedAt"),
  };
}
