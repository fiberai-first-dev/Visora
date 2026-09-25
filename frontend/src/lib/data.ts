import {
  ApiCompetitor,
  ApiContentGap,
  ApiFix,
  ApiIssue,
  ApiIssueGroup,
  ApiKeywordGap,
  ApiMetricsHistory,
  ApiMonitorEvent,
  ApiPage,
  ApiProject,
  ApiRecommendation,
  ApiSearchIntent,
  apiGetData,
  asArray,
  isRealTimestamp,
  normalizeCompetitor,
  normalizeContentGap,
  normalizeFix,
  normalizeIssue,
  normalizeIssueGroup,
  normalizeKeywordGap,
  normalizeMetricsHistory,
  normalizeMonitorEvent,
  normalizePage,
  normalizeProject,
  normalizeRecommendation,
  normalizeSearchIntent,
  normalizeSerpResult,
  pick,
  pickNumber,
  pickString,
} from "@/lib/api";

// Re-export the primitives so pages can import everything from one place.
export * from "@/lib/api";

/**
 * Every loader below returns rows the backend actually stored, or an empty
 * collection when there is genuinely nothing yet. No loader invents data: pages
 * render an explicit empty state instead.
 */

export async function listProjects(): Promise<ApiProject[]> {
  const raw = await apiGetData<unknown[]>("/projects");
  return asArray(raw)
    .map(normalizeProject)
    .filter((project): project is ApiProject => project !== null);
}

export async function getProject(
  id: number | string,
  headers?: Record<string, string>,
): Promise<ApiProject | null> {
  const raw = await apiGetData<unknown>(`/projects/${id}`, undefined, headers);
  return raw ? normalizeProject(raw) : null;
}

// ── Summary ───────────────────────────────────────────────────────────────────

export type ProjectSummary = {
  project: ApiProject | null;
  seo: {
    score: number | null;
    computedAt: string | null;
    breakdown: Record<string, unknown>;
    pagesCrawled: number;
    issuesCritical: number;
    issuesWarning: number;
    issuesNotice: number;
    lastCrawlAt: string | null;
    avgResponseMs: number | null;
  };
  geo: {
    score: number | null;
    mentionRate: number | null;
    citationRate: number | null;
    competitorSov: number | null;
    latestRunId: number | null;
    latestRunStatus: string | null;
    promptsTotal: number;
    promptsDone: number;
    promptsFailed: number;
  };
  counts: { pages: number; keywords: number; openRecommendations: number };
  activeJobs: { id: number; type: string; status: string }[];
};

export async function getProjectSummary(
  id: number | string,
  base?: string,
  headers?: Record<string, string>,
): Promise<ProjectSummary | null> {
  const raw = await apiGetData<unknown>(`/projects/${id}/summary`, base, headers);
  if (!raw) return null;

  const seo = pick<Record<string, unknown>>(raw, "seo") ?? {};
  const geo = pick<Record<string, unknown>>(raw, "geo") ?? {};
  const counts = pick<Record<string, unknown>>(raw, "counts") ?? {};
  const latestRun = pick<Record<string, unknown>>(seo, "latestRun");
  const issueCounts = pick<Record<string, unknown>>(seo, "issueCounts") ?? {};
  const geoRun = pick<Record<string, unknown>>(geo, "latestRun");

  return {
    project: normalizeProject(pick(raw, "project")),
    seo: {
      score: pickNumber(seo, "score"),
      computedAt: pickString(seo, "computedAt"),
      breakdown: pick<Record<string, unknown>>(seo, "breakdown") ?? {},
      pagesCrawled: pickNumber(latestRun, "pagesCrawled", "PagesCrawled") ?? 0,
      issuesCritical: pickNumber(issueCounts, "critical") ?? 0,
      issuesWarning: pickNumber(issueCounts, "warning") ?? 0,
      issuesNotice: pickNumber(issueCounts, "notice") ?? 0,
      lastCrawlAt:
        pickString(latestRun, "finishedAt", "FinishedAt") ??
        pickString(latestRun, "createdAt", "CreatedAt"),
      avgResponseMs: pickNumber(seo, "avgResponseMs", "avg_response_ms"),
    },
    geo: {
      score: pickNumber(geo, "score"),
      mentionRate: pickNumber(geo, "mentionRate"),
      citationRate: pickNumber(geo, "citationRate"),
      competitorSov: pickNumber(geo, "competitorSov"),
      latestRunId: pickNumber(geoRun, "ID", "id"),
      latestRunStatus: pickString(geoRun, "Status", "status"),
      promptsTotal: pickNumber(geoRun, "PromptsTotal", "promptsTotal") ?? 0,
      promptsDone: pickNumber(geoRun, "PromptsDone", "promptsDone") ?? 0,
      promptsFailed: pickNumber(geoRun, "PromptsFailed", "promptsFailed") ?? 0,
    },
    counts: {
      pages: pickNumber(counts, "pages") ?? 0,
      keywords: pickNumber(counts, "keywords") ?? 0,
      openRecommendations: pickNumber(counts, "openRecommendations") ?? 0,
    },
    activeJobs: asArray<unknown>(pick(raw, "activeJobs")).map((job) => ({
      id: pickNumber(job, "id", "ID") ?? 0,
      type: pickString(job, "type", "Type") ?? "job",
      status: pickString(job, "status", "Status") ?? "queued",
    })),
  };
}

/** Whether the project has ever produced a real measurement. */
export function hasMeasurement(summary: ProjectSummary | null): boolean {
  if (!summary) return false;
  return (
    summary.seo.score !== null ||
    summary.geo.score !== null ||
    isRealTimestamp(summary.seo.lastCrawlAt)
  );
}

// ── Collection loaders ────────────────────────────────────────────────────────

export async function getPages(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<ApiPage[]> {
  const raw = await apiGetData<unknown[]>(`/projects/${projectId}/pages`, undefined, headers);
  return asArray(raw).map(normalizePage);
}

export async function getIssues(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<ApiIssue[]> {
  const raw = await apiGetData<unknown[]>(`/projects/${projectId}/issues`, undefined, headers);
  return asArray(raw).map(normalizeIssue);
}

/** Count issues per page URL, used by the pages table and the overview. */
export function issueCountsByUrl(issues: ApiIssue[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const issue of issues) {
    if (!issue.url) continue;
    counts.set(issue.url, (counts.get(issue.url) ?? 0) + 1);
  }
  return counts;
}

export type SeoAudit = {
  score: number | null;
  computedAt: string | null;
  breakdown: Record<string, unknown>;
  issues: ApiIssueGroup[];
  totals: { issueTypes: number; affectedPages: number; bySeverity: Record<string, number> };
  pagesCrawled: number;
  crawlFinishedAt: string | null;
  avgResponseMs: number | null;
};

export async function getSeoAudit(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<SeoAudit | null> {
  const raw = await apiGetData<unknown>(`/projects/${projectId}/seo`, undefined, headers);
  if (!raw) return null;

  const totals = pick<Record<string, unknown>>(raw, "totals") ?? {};
  const run = pick<Record<string, unknown>>(raw, "crawl_run");
  const bySeverity = pick<Record<string, unknown>>(totals, "by_severity") ?? {};

  return {
    score: pickNumber(raw, "score"),
    computedAt: pickString(raw, "computed_at"),
    breakdown: pick<Record<string, unknown>>(raw, "breakdown") ?? {},
    issues: asArray<unknown>(pick(raw, "issues")).map(normalizeIssueGroup),
    totals: {
      issueTypes: pickNumber(totals, "issue_types") ?? 0,
      affectedPages: pickNumber(totals, "affected_pages") ?? 0,
      bySeverity: {
        critical: pickNumber(bySeverity, "critical") ?? 0,
        warning: pickNumber(bySeverity, "warning") ?? 0,
        notice: pickNumber(bySeverity, "notice") ?? 0,
      },
    },
    pagesCrawled: pickNumber(run, "PagesCrawled", "pagesCrawled") ?? 0,
    crawlFinishedAt: pickString(run, "FinishedAt", "finishedAt"),
    avgResponseMs: pickNumber(raw, "avg_response_ms", "avgResponseMs"),
  };
}

export async function getPageDetail(
  projectId: number | string,
  pageId: string,
  headers?: Record<string, string>,
) {
  const raw = await apiGetData<unknown>(`/projects/${projectId}/pages/${pageId}`, undefined, headers);
  if (!raw) return null;
  return {
    page: normalizePage(pick(raw, "page")),
    issues: asArray<unknown>(pick(raw, "issues")).map(normalizeIssue),
  };
}

export async function getIssueDetail(
  projectId: number | string,
  issueId: string,
  headers?: Record<string, string>,
) {
  const raw = await apiGetData<unknown>(`/projects/${projectId}/issues/${issueId}`, undefined, headers);
  if (!raw) return null;
  return {
    issue: normalizeIssue(pick(raw, "issue")),
    affectedUrls: asArray<string>(pick(raw, "affected_urls")),
    affectedCount: pickNumber(raw, "affected_count") ?? 0,
    page: pick(raw, "page") ? normalizePage(pick(raw, "page")) : null,
  };
}

export async function getSearchIntents(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<ApiSearchIntent[]> {
  const raw = await apiGetData<unknown[]>(`/projects/${projectId}/search-intents`, undefined, headers);
  return asArray(raw).map(normalizeSearchIntent);
}

export async function getSearchIntentDetail(
  projectId: number | string,
  intentId: string,
  headers?: Record<string, string>,
) {
  const raw = await apiGetData<unknown>(
    `/projects/${projectId}/search-intents/${intentId}`,
    undefined,
    headers,
  );
  if (!raw) return null;
  return {
    intent: normalizeSearchIntent(pick(raw, "intent")),
    query: pickString(raw, "query") ?? "",
    ownPosition: pickNumber(raw, "own_position"),
    results: asArray<unknown>(pick(raw, "results")).map(normalizeSerpResult),
    keywordGap: pick(raw, "keyword_gap") ? normalizeKeywordGap(pick(raw, "keyword_gap")) : null,
  };
}

export async function getKeywordGaps(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<ApiKeywordGap[]> {
  const raw = await apiGetData<unknown[]>(`/projects/${projectId}/keyword-gaps`, undefined, headers);
  return asArray(raw).map(normalizeKeywordGap);
}

export async function getContentGaps(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<ApiContentGap[]> {
  const raw = await apiGetData<unknown[]>(`/projects/${projectId}/content-gaps`, undefined, headers);
  return asArray(raw).map(normalizeContentGap);
}

export async function getCompetitors(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<ApiCompetitor[]> {
  const raw = await apiGetData<unknown[]>(`/projects/${projectId}/competitors`, undefined, headers);
  return asArray(raw).map(normalizeCompetitor);
}

export type CompetitorInsights = {
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

export async function getCompetitorDetail(
  projectId: number | string,
  compId: string,
  headers?: Record<string, string>,
) {
  const raw = await apiGetData<unknown>(
    `/projects/${projectId}/competitors/${compId}`,
    undefined,
    headers,
  );
  if (!raw) return null;

  const insightsRaw = pick<Record<string, unknown>>(raw, "insights") ?? {};
  const insights: CompetitorInsights = {
    keywordOverlap: pickNumber(insightsRaw, "keyword_overlap"),
    sharedQueryCount: pickNumber(insightsRaw, "shared_query_count") ?? 0,
    ownQueryCount: pickNumber(insightsRaw, "own_query_count") ?? 0,
    aiVisibility: pickNumber(insightsRaw, "ai_visibility"),
    aiMentions: pickNumber(insightsRaw, "ai_mentions") ?? 0,
    aiPromptsChecked: pickNumber(insightsRaw, "ai_prompts_checked") ?? 0,
    topSharedTopics: asArray<string>(pick(insightsRaw, "top_shared_topics")),
    trackedPages: pickNumber(insightsRaw, "tracked_pages") ?? 0,
    trackedQueries: pickNumber(insightsRaw, "tracked_queries") ?? 0,
    crawlStatus: pickString(insightsRaw, "crawl_status") ?? "pending",
  };

  return {
    competitor: normalizeCompetitor(pick(raw, "competitor")),
    relationshipType: pickString(pick(raw, "relationship"), "Type", "type") ?? "unknown",
    insights,
    contentGaps: asArray<unknown>(pick(raw, "content_gaps")).map((gap) => ({
      id: pickNumber(gap, "id") ?? 0,
      topic: pickString(gap, "topic") ?? "",
      intent: pickString(gap, "intent") ?? "",
      priority: pickString(gap, "priority") ?? "medium",
      evidence: asArray<string>(pick(gap, "evidence")),
      yourPages: asArray<{ url?: string; title?: string }>(pick(gap, "yourPages")).map((page) => ({
        url: page?.url ?? "",
        title: page?.title ?? "",
      })),
    })),
    pages: asArray<unknown>(pick(raw, "pages")).map(normalizePage),
  };
}

export type SeoComparisonRow = {
  domain: string;
  brandName: string;
  isBrand: boolean;
  classification: string;
  relationshipType: string;
  pagesCrawled: number;
  productPages: number;
  collectionPages: number;
  contentPages: number;
  schemaCoverage: number;
  avgWordCount: number;
  missingTitles: number;
  missingMeta: number;
  imagesMissingAlt: number;
  brokenPages: number;
};

export async function getSeoComparison(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<SeoComparisonRow[]> {
  const raw = await apiGetData<{ comparisons?: unknown[] }>(
    `/projects/${projectId}/seo-comparison`,
    undefined,
    headers,
  );
  return asArray<unknown>(pick(raw, "comparisons")).map((row) => ({
    domain: pickString(row, "domain") ?? "",
    brandName: pickString(row, "brand_name") ?? "",
    isBrand: pick(row, "is_brand") === true,
    classification: pickString(row, "classification") ?? "",
    relationshipType: pickString(row, "relationship_type") ?? "",
    pagesCrawled: pickNumber(row, "pages_crawled") ?? 0,
    productPages: pickNumber(row, "product_pages") ?? 0,
    collectionPages: pickNumber(row, "collection_pages") ?? 0,
    contentPages: pickNumber(row, "content_pages") ?? 0,
    schemaCoverage: pickNumber(row, "schema_coverage") ?? 0,
    avgWordCount: pickNumber(row, "avg_word_count") ?? 0,
    missingTitles: pickNumber(row, "missing_titles") ?? 0,
    missingMeta: pickNumber(row, "missing_meta_descriptions") ?? 0,
    imagesMissingAlt: pickNumber(row, "images_missing_alt") ?? 0,
    brokenPages: pickNumber(row, "broken_pages") ?? 0,
  }));
}

export type PerformanceReport = {
  totals: {
    pages: number;
    avgMs: number;
    medianMs: number;
    p90Ms: number;
    maxMs: number;
    totalBytes: number;
    images: number;
    imagesMissingAlt: number;
    brokenPages: number;
  };
  byPageType: { pageType: string; pages: number; avgMs: number; maxMs: number; totalBytes: number }[];
  slowestPages: {
    url: string;
    pageType: string;
    status: number | null;
    responseMs: number;
    responseBytes: number;
    title: string;
  }[];
  crawlStartedAt: string | null;
  crawlFinishedAt: string | null;
  note: string;
  hasRun: boolean;
};

export async function getPerformance(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<PerformanceReport | null> {
  const raw = await apiGetData<unknown>(`/projects/${projectId}/performance`, undefined, headers);
  if (!raw) return null;

  const totals = pick<Record<string, unknown>>(raw, "totals") ?? {};
  const run = pick(raw, "crawl_run");

  return {
    totals: {
      pages: pickNumber(totals, "pages") ?? 0,
      avgMs: pickNumber(totals, "avg_ms") ?? 0,
      medianMs: pickNumber(totals, "median_ms") ?? 0,
      p90Ms: pickNumber(totals, "p90_ms") ?? 0,
      maxMs: pickNumber(totals, "max_ms") ?? 0,
      totalBytes: pickNumber(totals, "total_bytes") ?? 0,
      images: pickNumber(totals, "images") ?? 0,
      imagesMissingAlt: pickNumber(totals, "images_missing_alt") ?? 0,
      brokenPages: pickNumber(totals, "broken_pages") ?? 0,
    },
    byPageType: asArray<unknown>(pick(raw, "by_page_type")).map((row) => ({
      pageType: pickString(row, "page_type") ?? "other",
      pages: pickNumber(row, "pages") ?? 0,
      avgMs: pickNumber(row, "avg_ms") ?? 0,
      maxMs: pickNumber(row, "max_ms") ?? 0,
      totalBytes: pickNumber(row, "total_bytes") ?? 0,
    })),
    slowestPages: asArray<unknown>(pick(raw, "slowest_pages")).map((row) => ({
      url: pickString(row, "url") ?? "",
      pageType: pickString(row, "page_type") ?? "other",
      status: pickNumber(row, "status"),
      responseMs: pickNumber(row, "response_ms") ?? 0,
      responseBytes: pickNumber(row, "response_bytes") ?? 0,
      title: pickString(row, "title") ?? "",
    })),
    crawlStartedAt: pickString(raw, "crawl_started_at"),
    crawlFinishedAt: pickString(raw, "crawl_finished_at"),
    note: pickString(raw, "measurement_note") ?? "",
    hasRun: run !== null && run !== undefined,
  };
}

export async function getRecommendations(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<ApiRecommendation[]> {
  const raw = await apiGetData<unknown[]>(
    `/projects/${projectId}/recommendations`,
    undefined,
    headers,
  );
  return asArray(raw).map(normalizeRecommendation);
}

export async function getFixes(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<ApiFix[]> {
  const raw = await apiGetData<unknown[]>(`/projects/${projectId}/fixes`, undefined, headers);
  return asArray(raw).map(normalizeFix);
}

export async function getMonitoringEvents(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<ApiMonitorEvent[]> {
  const raw = await apiGetData<unknown[]>(`/projects/${projectId}/monitoring`, undefined, headers);
  return asArray(raw).map(normalizeMonitorEvent);
}

export async function getMetricsHistory(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<ApiMetricsHistory[]> {
  const raw = await apiGetData<unknown[]>(
    `/projects/${projectId}/metrics/history`,
    undefined,
    headers,
  );
  return asArray(raw).map(normalizeMetricsHistory);
}

export async function getKeywords(projectId: number | string, headers?: Record<string, string>) {
  const raw = await apiGetData<unknown[]>(`/projects/${projectId}/keywords`, undefined, headers);
  return asArray(raw).map((row) => ({
    id: pickNumber(row, "id", "ID") ?? 0,
    keyword: pickString(row, "keyword", "Keyword") ?? "",
    intent: pickString(row, "intent", "Intent") ?? "",
    source: pickString(row, "source", "Source") ?? "",
    status: pickString(row, "status", "Status") ?? "",
  }));
}

export type GeoPromptAnswer = {
  model: string;
  text: string;
  namedYou: boolean;
  namedBrands: string[];
};

export type GeoPromptVisibility = {
  promptId: number;
  text: string;
  intent: string;
  topic: string;
  modelsChecked: string[];
  modelsMentioned: string[];
  mentioned: boolean;
  mentionCount: number;
  bestCompetitor: string;
  competitorMentions: number;
  gap: string;
  answers: GeoPromptAnswer[];
};

export type GeoModelStat = {
  model: string;
  checked: number;
  named: number;
  rate: number | null;
};

export type GeoVisibility = {
  geoScore: number | null;
  mentionRate: number | null;
  citationRate: number | null;
  competitorSov: number | null;
  promptsChecked: number;
  promptsMentioned: number;
  answersCollected: number;
  answersMentioned: number;
  perPrompt: GeoPromptVisibility[];
  competitorMentions: { brand: string; count: number }[];
  modelStats: GeoModelStat[];
};

/** AI / GEO visibility — where assistants mention you for buyer questions. */
export async function getGeoVisibility(
  projectId: number | string,
  headers?: Record<string, string>,
): Promise<GeoVisibility> {
  const raw = await apiGetData<unknown>(`/projects/${projectId}/geo/overview`, undefined, headers);
  if (!raw) {
    return {
      geoScore: null,
      mentionRate: null,
      citationRate: null,
      competitorSov: null,
      promptsChecked: 0,
      promptsMentioned: 0,
      answersCollected: 0,
      answersMentioned: 0,
      perPrompt: [],
      competitorMentions: [],
      modelStats: [],
    };
  }

  const metrics = pick<Record<string, unknown>>(raw, "metrics") ?? {};
  return {
    geoScore: pickNumber(metrics, "geoScore", "GeoScore"),
    mentionRate: pickNumber(metrics, "mentionRate", "MentionRate"),
    citationRate: pickNumber(metrics, "citationRate", "CitationRate"),
    competitorSov: pickNumber(metrics, "competitorSov", "CompetitorSov"),
    promptsChecked: pickNumber(raw, "promptsChecked") ?? 0,
    promptsMentioned: pickNumber(raw, "promptsMentioned") ?? 0,
    answersCollected: pickNumber(raw, "answersCollected") ?? 0,
    answersMentioned: pickNumber(raw, "answersMentioned") ?? 0,
    perPrompt: asArray<unknown>(pick(raw, "perPrompt")).map((row) => ({
      promptId: pickNumber(row, "promptId", "PromptID") ?? 0,
      text: pickString(row, "text", "Text") ?? "",
      intent: pickString(row, "intent", "Intent") ?? "commercial",
      topic: pickString(row, "topic", "Topic") ?? "",
      modelsChecked: asArray<string>(pick(row, "modelsChecked")),
      modelsMentioned: asArray<string>(pick(row, "modelsMentioned")),
      mentioned: pick(row, "mentioned") === true,
      mentionCount: pickNumber(row, "mentionCount") ?? 0,
      bestCompetitor: pickString(row, "bestCompetitor") ?? "",
      competitorMentions: pickNumber(row, "competitorMentions") ?? 0,
      gap: pickString(row, "gap") ?? "missing",
      answers: asArray<unknown>(pick(row, "answers")).map((a) => ({
        model: pickString(a, "model") ?? "",
        text: pickString(a, "text") ?? "",
        namedYou: pick(a, "namedYou") === true,
        namedBrands: asArray<string>(pick(a, "namedBrands")),
      })),
    })),
    competitorMentions: asArray<unknown>(pick(raw, "competitorMentions")).map((row) => ({
      brand: pickString(row, "brand", "Brand") ?? "",
      count: pickNumber(row, "count", "Count") ?? 0,
    })),
    modelStats: asArray<unknown>(pick(raw, "modelStats")).map((row) => ({
      model: pickString(row, "model") ?? "",
      checked: pickNumber(row, "checked") ?? 0,
      named: pickNumber(row, "named") ?? 0,
      rate: pickNumber(row, "rate"),
    })),
  };
}
