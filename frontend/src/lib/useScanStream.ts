"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { PUBLIC_API_BASE, apiGetData, asArray, pick, pickNumber, pickString } from "@/lib/api";
import {
  CompetitorSample,
  GeoModelReply,
  GeoPromptSample,
  JourneyStepId,
  SerpCheckSample,
  journeyStepFromStage,
} from "@/lib/journey";

export type ScanEventType = "progress" | "milestone" | "error" | "complete";

/** One real event from the backend pipeline (GET /projects/:id/scan-stream). */
export type ScanEvent = {
  id: number;
  stage: number;
  eventType: ScanEventType;
  message: string;
  data: Record<string, unknown>;
  receivedAt: string;
};

/** A page the crawler actually fetched. */
export type CrawledPage = {
  url: string;
  path: string;
  pageType: string;
  status: number | null;
  title?: string;
  description?: string;
  h1?: string;
  ogImage?: string;
  responseMs?: number;
  wordCount?: number;
  at: string;
};

export type ScanSnapshot = {
  connected: boolean;
  running: boolean;
  finished: boolean;
  stage: number;
  journeyStep: JourneyStepId;
  events: ScanEvent[];
  values: Record<string, unknown>;
  crawledPages: CrawledPage[];
  serpChecks: SerpCheckSample[];
  geoPrompts: GeoPromptSample[];
  geoReplies: GeoModelReply[];
  competitors: CompetitorSample[];
  errorMessages: string[];
  recentJobErrors: { type: string; error: string }[];
  activeJobs: { id: number; type: string; status: string }[];
  /** Set when the worker gave up on a step after its retries (stage 0 error). */
  failedMessage: string | null;
};

/** Human labels for the 16 real pipeline stages (see arch.md). */
export const STAGE_LABELS: Record<number, string> = {
  1: "Looking at your site",
  2: "Checking site quality",
  3: "Understanding your brand",
  4: "Finding buyer searches",
  5: "Checking Google",
  6: "Finding who else shows up",
  7: "Looking at rival sites",
  8: "Comparing coverage",
  9: "Asking AI assistants",
  10: "Reading AI answers",
  11: "Prioritising what matters",
  12: "Writing what to fix",
  13: "Preparing fixes",
  14: "Ready to apply",
  15: "Preparing a re-check",
  16: "Measuring improvement",
};

export const TOTAL_STAGES = 16;
export const FINAL_STAGE = 16;

const MAX_EVENTS = 400;
const MAX_CRAWLED_PAGES = 120;
const MAX_SERP_CHECKS = 40;
const MAX_GEO_PROMPTS = 24;
const MAX_GEO_REPLIES = 48;

const EMPTY: ScanSnapshot = {
  connected: false,
  running: false,
  finished: false,
  stage: 0,
  journeyStep: 1,
  events: [],
  values: {},
  crawledPages: [],
  serpChecks: [],
  geoPrompts: [],
  geoReplies: [],
  competitors: [],
  errorMessages: [],
  recentJobErrors: [],
  activeJobs: [],
  failedMessage: null,
};

function parseSerpResults(raw: unknown): SerpCheckSample["results"] {
  if (!Array.isArray(raw)) return [];
  const result: SerpCheckSample["results"] = [];
  for (const row of raw) {
    if (!row || typeof row !== "object") continue;
    const item = row as Record<string, unknown>;
    const domain = pickString(item, "domain") ?? "";
    if (!domain) continue;
    const url = pickString(item, "url");
    const snippet = pickString(item, "snippet");
    result.push({
      position: pickNumber(item, "position") ?? 0,
      domain,
      title: pickString(item, "title") ?? domain,
      ...(url ? { url } : {}),
      ...(snippet ? { snippet } : {}),
      own: Boolean(item.own),
    });
  }
  return result;
}

function parseCompetitors(raw: unknown): CompetitorSample[] {
  if (!Array.isArray(raw)) return [];
  const result: CompetitorSample[] = [];
  for (const row of raw) {
    if (!row || typeof row !== "object") continue;
    const item = row as Record<string, unknown>;
    const domain = pickString(item, "domain") ?? "";
    if (!domain) continue;
    result.push({
      domain,
      brandName: pickString(item, "brand_name") ?? undefined,
      classification: pickString(item, "classification") ?? undefined,
      appearances: pickNumber(item, "appearances") ?? undefined,
      bestPosition: pickNumber(item, "best_position") ?? undefined,
    });
  }
  return result;
}

/**
 * Consumes the real scan event stream plus live job status.
 *
 * Nothing here is synthesised: `values` holds the newest payload the backend
 * actually sent, and `crawledPages` holds the URLs the crawler actually stored.
 *
 * Pass `enabled: false` until a new scan is triggered so we never replay an
 * old stage-16 complete before the POST lands.
 */
export function useScanStream(projectId: string, enabled = true) {
  const [snapshot, setSnapshot] = useState<ScanSnapshot>(EMPTY);
  const [retryToken, setRetryToken] = useState(0);

  const seenIds = useRef<Set<number>>(new Set());
  const lastEventAt = useRef<number>(0);

  const applyEvent = useCallback((incoming: ScanEvent) => {
    if (Number.isFinite(incoming.id) && incoming.id > 0) {
      if (seenIds.current.has(incoming.id)) return;
      seenIds.current.add(incoming.id);
    }

    setSnapshot((prev) => {
      const events = [...prev.events, incoming].slice(-MAX_EVENTS);
      const values = { ...prev.values, ...incoming.data };

      let crawledPages = prev.crawledPages;
      const url = pickString(incoming.data, "url");
      if (url && incoming.stage === 1) {
        // The crawl's "Crawling <site>" event carries the start URL with no
        // content; merge by URL so the fetched page's fields fill it in.
        const next: CrawledPage = {
          url,
          path: pickString(incoming.data, "path") ?? url,
          pageType: pickString(incoming.data, "page_type") ?? "other",
          status: pickNumber(incoming.data, "status"),
          title: pickString(incoming.data, "title") || undefined,
          ogImage: pickString(incoming.data, "og_image") || undefined,
          description: pickString(incoming.data, "description") || undefined,
          h1: pickString(incoming.data, "h1") || undefined,
          responseMs: pickNumber(incoming.data, "response_ms") ?? undefined,
          wordCount: pickNumber(incoming.data, "word_count") ?? undefined,
          at: incoming.receivedAt,
        };
        const idx = crawledPages.findIndex((p) => p.url === url);
        if (idx >= 0) {
          const prevPage = crawledPages[idx];
          crawledPages = crawledPages.slice();
          crawledPages[idx] = {
            ...next,
            status: next.status ?? prevPage.status,
            title: next.title ?? prevPage.title,
            ogImage: next.ogImage ?? prevPage.ogImage,
            description: next.description ?? prevPage.description,
            h1: next.h1 ?? prevPage.h1,
            responseMs: next.responseMs ?? prevPage.responseMs,
            wordCount: next.wordCount ?? prevPage.wordCount,
          };
        } else {
          crawledPages = [...crawledPages, next].slice(-MAX_CRAWLED_PAGES);
        }
      }

      let serpChecks = prev.serpChecks;
      const query = pickString(incoming.data, "query");
      if (incoming.stage === 5 && query) {
        const sample: SerpCheckSample = {
          query,
          brandPosition: pickNumber(incoming.data, "brand_position") ?? 0,
          foundInTop50: Boolean(incoming.data.found_in_top_50),
          results: parseSerpResults(incoming.data.results),
          queriesChecked: pickNumber(incoming.data, "queries_checked") ?? serpChecks.length + 1,
          top10: pickNumber(incoming.data, "top_10_appearances") ?? 0,
          top50: pickNumber(incoming.data, "top_50_appearances") ?? 0,
          notFound: pickNumber(incoming.data, "not_found") ?? 0,
        };
        const existingIdx = serpChecks.findIndex((item) => item.query === query);
        if (existingIdx >= 0) {
          serpChecks = serpChecks.slice();
          serpChecks[existingIdx] = sample;
        } else {
          serpChecks = [...serpChecks, sample].slice(-MAX_SERP_CHECKS);
        }
      }

      let geoPrompts = prev.geoPrompts;
      let geoReplies = prev.geoReplies;
      const prompt = pickString(incoming.data, "prompt");
      if (incoming.stage === 9 && prompt) {
        const sample: GeoPromptSample = {
          prompt,
          index: pickNumber(incoming.data, "prompt_index") ?? geoPrompts.length + 1,
          total: pickNumber(incoming.data, "prompts_total") ?? 0,
          models: Array.isArray(incoming.data.models)
            ? incoming.data.models.filter((item): item is string => typeof item === "string")
            : ["chatgpt", "claude", "gemini", "grok"],
        };
        geoPrompts = [...geoPrompts.filter((item) => item.prompt !== prompt), sample].slice(-MAX_GEO_PROMPTS);

        const model = pickString(incoming.data, "model");
        if (model) {
          const reply: GeoModelReply = {
            model: model.toLowerCase(),
            prompt,
            response: pickString(incoming.data, "response") ?? "",
            failed: Boolean(incoming.data.failed),
            at: incoming.receivedAt,
          };
          const key = `${reply.model}::${reply.prompt}`;
          geoReplies = [
            ...geoReplies.filter((item) => `${item.model}::${item.prompt}` !== key),
            reply,
          ].slice(-MAX_GEO_REPLIES);
        }
      }

      let competitors = prev.competitors;
      if (incoming.stage === 6 || incoming.stage === 7) {
        const classified = parseCompetitors(incoming.data.competitors);
        if (classified.length > 0) {
          competitors = classified;
        } else {
          const domains = asArray<unknown>(incoming.data.domains)
            .map((item) => (typeof item === "string" ? item : pickString(item, "domain")))
            .filter((item): item is string => !!item);
          const domain = pickString(incoming.data, "domain");
          if (domain) domains.push(domain);
          if (domains.length > 0) {
            const byDomain = new Map(competitors.map((item) => [item.domain, item]));
            for (const d of domains) {
              if (!byDomain.has(d)) byDomain.set(d, { domain: d, classification: "Classifying…" });
            }
            competitors = Array.from(byDomain.values());
          }
        }
      }

      const errorMessages =
        incoming.eventType === "error"
          ? [...prev.errorMessages, incoming.message].slice(-10)
          : prev.errorMessages;

      const finished = incoming.eventType === "complete" && incoming.stage === FINAL_STAGE;
      const stage = Math.max(prev.stage, incoming.stage);
      const failedMessage =
        incoming.eventType === "error" && incoming.stage === 0 ? incoming.message : prev.failedMessage;

      return {
        ...prev,
        failedMessage,
        events,
        values,
        crawledPages,
        serpChecks,
        geoPrompts,
        geoReplies,
        competitors,
        errorMessages,
        stage,
        finished: prev.finished || finished,
        running: finished || failedMessage ? false : true,
        journeyStep: journeyStepFromStage(stage, prev.finished || finished),
      };
    });
  }, []);

  useEffect(() => {
    if (!enabled || typeof window === "undefined") return;

    seenIds.current = new Set();
    lastEventAt.current = Date.now();
    setSnapshot({ ...EMPTY, running: true });

    let source: EventSource | null = null;
    try {
      source = new EventSource(`${PUBLIC_API_BASE}/projects/${projectId}/scan-stream`);
    } catch {
      return;
    }

    source.onopen = () => setSnapshot((prev) => ({ ...prev, connected: true, running: true }));

    source.onmessage = (message) => {
      try {
        const parsed = JSON.parse(message.data) as Record<string, unknown>;
        const stage = pickNumber(parsed, "stage") ?? 0;
        const rawType = (pickString(parsed, "event_type", "eventType") ?? "progress").toLowerCase();
        if (stage < 0 || (stage === 0 && rawType !== "error")) return;

        lastEventAt.current = Date.now();
        const eventType: ScanEventType =
          rawType === "complete" || rawType === "error" || rawType === "milestone"
            ? (rawType as ScanEventType)
            : "progress";

        applyEvent({
          id: pickNumber(parsed, "id") ?? 0,
          stage,
          eventType,
          message: pickString(parsed, "message") ?? "",
          data: (pick<Record<string, unknown>>(parsed, "data") ?? {}) as Record<string, unknown>,
          receivedAt: new Date().toLocaleTimeString("en-US", { hour12: false }),
        });
      } catch {
        // Ignore malformed frames; the stream keeps running.
      }
    };

    source.onerror = () => {
      setSnapshot((prev) => ({ ...prev, connected: false }));
    };

    return () => {
      source?.close();
    };
  }, [projectId, retryToken, applyEvent, enabled]);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;

    const poll = async () => {
      const live = await apiGetData<Record<string, unknown>>(`/projects/${projectId}/live-status`);
      if (cancelled || !live) return;

      const jobs = asArray<unknown>(pick(live, "active_jobs")).map((job) => ({
        id: pickNumber(job, "id", "ID") ?? 0,
        type: pickString(job, "type", "Type") ?? "job",
        status: pickString(job, "status", "Status") ?? "queued",
      }));

      const recentJobErrors = asArray<unknown>(pick(live, "recent_errors"))
        .map((row) => ({
          type: pickString(row, "type") ?? "job",
          error: pickString(row, "error") ?? "",
        }))
        .filter((row) => row.error.length > 0);

      setSnapshot((prev) => {
        if (prev.finished || prev.failedMessage) {
          return { ...prev, activeJobs: jobs, recentJobErrors, running: false };
        }
        if (jobs.length > 0) {
          return { ...prev, activeJobs: jobs, recentJobErrors, running: true };
        }

        // Only the final stage means done; recommendations alone are not
        // enough because the AI fix step may still fail.
        if (prev.stage >= FINAL_STAGE) {
          return {
            ...prev,
            activeJobs: jobs,
            recentJobErrors: [],
            running: false,
            finished: true,
          };
        }

        // Quiet window: brand/LLM steps can take a while; don't false-pause
        // while the worker is still claimed on a long job.
        const quietForMs = Date.now() - lastEventAt.current;
        const stopped = prev.events.length > 0 && quietForMs > 90000 && recentJobErrors.length > 0;
        return {
          ...prev,
          activeJobs: jobs,
          recentJobErrors,
          running: stopped ? false : prev.running || quietForMs < 90000,
        };
      });
    };

    void poll();
    const interval = setInterval(poll, 5000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [projectId, retryToken, enabled]);

  const retry = useCallback(() => {
    seenIds.current = new Set();
    lastEventAt.current = Date.now();
    setSnapshot(EMPTY);
    setRetryToken((token) => token + 1);
  }, []);

  return useMemo(() => ({ ...snapshot, retry }), [snapshot, retry]);
}

/** Read a numeric value from the accumulated scan payload. */
export function scanNumber(values: Record<string, unknown>, ...keys: string[]): number | null {
  return pickNumber(values, ...keys);
}

/** Read a string value from the accumulated scan payload. */
export function scanString(values: Record<string, unknown>, ...keys: string[]): string | null {
  return pickString(values, ...keys);
}

/** Read a string list from the accumulated scan payload. */
export function scanStringList(values: Record<string, unknown>, ...keys: string[]): string[] {
  for (const key of keys) {
    const value = values[key];
    if (Array.isArray(value)) {
      return value.filter((item): item is string => typeof item === "string");
    }
  }
  return [];
}

/** Read a record list from the accumulated scan payload. */
export function scanRecords(values: Record<string, unknown>, ...keys: string[]): Record<string, unknown>[] {
  for (const key of keys) {
    const value = values[key];
    if (Array.isArray(value)) {
      return value.filter((item): item is Record<string, unknown> => !!item && typeof item === "object");
    }
  }
  return [];
}
