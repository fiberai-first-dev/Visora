import { SERVER_API_BASE, asArray, normalizeProject, type ApiProject } from "@/lib/api";
import { authHeaders } from "@/lib/auth";
import {
  getCompetitorDetail as loadCompetitorDetail,
  getCompetitors as loadCompetitors,
  getFixes as loadFixes,
  getGeoVisibility as loadGeoVisibility,
  getIssues as loadIssues,
  getKeywordGaps as loadKeywordGaps,
  getPageDetail as loadPageDetail,
  getPages as loadPages,
  getProject as loadProject,
  getProjectSummary as loadSummary,
  getRecommendations as loadRecommendations,
  getSearchIntentDetail as loadSearchIntentDetail,
  getSearchIntents as loadSearchIntents,
  getSeoAudit as loadSeoAudit,
  getSeoComparison as loadSeoComparison,
} from "@/lib/data";

/** Server-only: forwards the session cookie to the Go API. */
export async function listProjects(): Promise<ApiProject[]> {
  try {
    const res = await fetch(`${SERVER_API_BASE}/projects`, {
      cache: "no-store",
      headers: await authHeaders(),
    });
    if (!res.ok) return [];
    return asArray(await res.json())
      .map(normalizeProject)
      .filter((project): project is ApiProject => project !== null);
  } catch {
    return [];
  }
}

export async function getProject(id: number | string) {
  return loadProject(id, await authHeaders());
}

export async function getProjectSummary(id: number | string) {
  return loadSummary(id, undefined, await authHeaders());
}

export async function getPages(id: number | string) {
  return loadPages(id, await authHeaders());
}

export async function getIssues(id: number | string) {
  return loadIssues(id, await authHeaders());
}

export async function getSeoAudit(id: number | string) {
  return loadSeoAudit(id, await authHeaders());
}

export async function getPageDetail(id: number | string, pageId: string) {
  return loadPageDetail(id, pageId, await authHeaders());
}

export async function getSearchIntents(id: number | string) {
  return loadSearchIntents(id, await authHeaders());
}

export async function getSearchIntentDetail(id: number | string, intentId: string) {
  return loadSearchIntentDetail(id, intentId, await authHeaders());
}

export async function getKeywordGaps(id: number | string) {
  return loadKeywordGaps(id, await authHeaders());
}

export async function getCompetitors(id: number | string) {
  return loadCompetitors(id, await authHeaders());
}

export async function getCompetitorDetail(id: number | string, compId: string) {
  return loadCompetitorDetail(id, compId, await authHeaders());
}

export async function getSeoComparison(id: number | string) {
  return loadSeoComparison(id, await authHeaders());
}

export async function getRecommendations(id: number | string) {
  return loadRecommendations(id, await authHeaders());
}

export async function getFixes(id: number | string) {
  return loadFixes(id, await authHeaders());
}

export async function getGeoVisibility(id: number | string) {
  return loadGeoVisibility(id, await authHeaders());
}
