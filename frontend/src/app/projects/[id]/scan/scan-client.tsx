"use client";

import React, { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import { PUBLIC_API_BASE, apiGetData, asArray, pick, pickString } from "@/lib/api";
import { ProjectSummary, getProjectSummary } from "@/lib/data";
import { useScanStream, scanNumber, scanString } from "@/lib/useScanStream";
import { JourneyStepId } from "@/lib/journey";
import { ScanExperience } from "@/components/scan/ScanExperience";
import { HealthPanel } from "@/components/scan/panels/HealthPanel";
import { TechnicalPanel } from "@/components/scan/panels/TechnicalPanel";
import { GooglePanel } from "@/components/scan/panels/GooglePanel";
import { CompetitorsPanel } from "@/components/scan/panels/CompetitorsPanel";
import { AiPanel } from "@/components/scan/panels/AiPanel";
import { ReadyPanel } from "@/components/scan/panels/ReadyPanel";

async function triggerScan(projectId: string) {
  const res = await fetch(`${PUBLIC_API_BASE}/projects/${projectId}/scan`, {
    method: "POST",
    credentials: "include",
  });
  if (!res.ok) throw new Error(`Scan trigger failed (${res.status})`);
}

async function hasActiveJobs(projectId: string): Promise<boolean> {
  const live = await apiGetData<Record<string, unknown>>(
    `/projects/${projectId}/live-status`,
    PUBLIC_API_BASE,
  );
  if (!live) return false;
  const jobs = asArray(pick(live, "active_jobs"));
  return jobs.some((job) => {
    const status = (pickString(job, "status", "Status") ?? "").toLowerCase();
    return status === "queued" || status === "running";
  });
}

export function ScanClient({
  projectId,
  domain,
  initialSummary,
}: {
  projectId: string;
  domain: string;
  initialSummary: ProjectSummary | null;
}) {
  const searchParams = useSearchParams();
  const forceRerun = searchParams.get("rerun") === "1";
  const bootKey = searchParams.get("t") ?? (forceRerun ? "rerun" : "open");

  const [streamReady, setStreamReady] = useState(false);
  const [bootError, setBootError] = useState<string | null>(null);
  const [step, setStep] = useState<JourneyStepId>(1);
  const [healthDone, setHealthDone] = useState(false);
  const [googleDone, setGoogleDone] = useState(false);
  const [aiDone, setAiDone] = useState(false);

  const scan = useScanStream(projectId, streamReady);
  const rivalsChecked = scanNumber(scan.values, "competitors_crawled") != null;
  const [summary, setSummary] = useState<ProjectSummary | null>(initialSummary);
  const [resuming, setResuming] = useState(false);

  // Always start (or attach to) a real pipeline when landing on /scan.
  // Without this, Re-run only opened the UI and replayed the last finished run.
  useEffect(() => {
    let cancelled = false;
    setStreamReady(false);
    setBootError(null);
    setStep(1);
    setHealthDone(false);
    setGoogleDone(false);
    setAiDone(false);

    (async () => {
      try {
        const alreadyRunning = forceRerun ? false : await hasActiveJobs(projectId);
        if (!alreadyRunning) {
          await triggerScan(projectId);
        }
        if (cancelled) return;
        setBootError(null);
        setStreamReady(true);
      } catch (err) {
        if (cancelled) return;
        setBootError(err instanceof Error ? err.message : "Could not start scan");
        // Still open the stream so a manually queued job can appear.
        setStreamReady(true);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [projectId, forceRerun, bootKey]);

  useEffect(() => {
    if (!scan.finished) return;
    let cancelled = false;
    void getProjectSummary(projectId, PUBLIC_API_BASE).then((next) => {
      if (!cancelled && next) setSummary(next);
    });
    return () => {
      cancelled = true;
    };
  }, [projectId, scan.finished]);

  const hasResults =
    scan.finished ||
    (scanNumber(scan.values, "recommendations") ?? 0) > 0 ||
    (Array.isArray(scan.values.top) && scan.values.top.length > 0);
  const stoppedEarly =
    !hasResults && !scan.running && !scan.finished && scan.events.length > 0;
  // Play every chapter in order. Backend stage must not skip AI answers.
  useEffect(() => {
    if (step >= 6) return;
    // Panels report when their animation finished; the long values are only a
    // safety net so a stalled stream can't freeze the journey.
    const waitMs =
      step === 1 ? (healthDone ? 300 : 45000) :
      step === 2 ? 8000 :
      step === 3 ? (googleDone ? 3000 : 32000) :
      step === 4 ? (aiDone ? 1500 : 80000) :
      rivalsChecked ? 9000 : 45000;
    const from = step;
    const id = window.setTimeout(() => {
      setStep((current) => (current === from ? ((from + 1) as JourneyStepId) : current));
    }, waitMs);
    return () => window.clearTimeout(id);
  }, [step, healthDone, googleDone, aiDone, rivalsChecked]);

  // Keep the visual journey on its own clock. A finished backend must not
  // skip homepage scroll, mini pages, or the four AI cards.

  const jobError = scan.recentJobErrors[0];
  const pauseDetail = bootError
    ? bootError
    : scan.failedMessage
      ? "The AI could not finish a step after several tries. Restart the scan to try again."
    : jobError?.error
      ? "A check could not finish. You can restart the scan."
      : scan.errorMessages.length > 0
        ? scan.errorMessages[scan.errorMessages.length - 1]
        : "The scan stopped before finishing.";

  const subtasks = useMemo(() => buildSubtasks(scan, step, domain), [scan, step, domain]);

  const resumeScan = async () => {
    setResuming(true);
    setBootError(null);
    try {
      await triggerScan(projectId);
      setStep(1);
      setHealthDone(false);
      setGoogleDone(false);
      setAiDone(false);
      scan.retry();
      setStreamReady(true);
    } catch (err) {
      setBootError(err instanceof Error ? err.message : "Could not restart scan");
    } finally {
      setResuming(false);
    }
  };

  return (
    <ScanExperience
      domain={domain}
      step={step}
      finished={scan.finished}
      running={scan.running || (!streamReady && !bootError)}
      stopped={
        Boolean(scan.failedMessage) ||
        (step >= 6 && !scan.finished && (stoppedEarly || Boolean(bootError)))
      }
      pauseDetail={pauseDetail}
      onRestart={() => void resumeScan()}
      restarting={resuming}
      subtasks={subtasks}
    >
      {!streamReady && !bootError ? (
        <div className="mx-auto flex max-w-md flex-col items-center py-16 text-center">
          <div className="mb-4 flex gap-1.5">
            <span className="ai-dot" />
            <span className="ai-dot" style={{ animationDelay: "0.15s" }} />
            <span className="ai-dot" style={{ animationDelay: "0.3s" }} />
          </div>
          <p className="font-[family-name:var(--font-display)] text-xl text-[#1a1a18]">
            Starting analysis…
          </p>
          <p className="mt-2 text-sm text-zinc-500">Queuing a fresh crawl of {domain}</p>
        </div>
      ) : null}
      {streamReady && step === 1 ? (
        <HealthPanel
          projectId={projectId}
          domain={domain}
          pages={scan.crawledPages}
          values={scan.values}
          onComplete={() => setHealthDone(true)}
        />
      ) : null}
      {streamReady && step === 2 ? (
        <TechnicalPanel domain={domain} pages={scan.crawledPages} values={scan.values} />
      ) : null}
      {streamReady && step === 3 ? (
        <GooglePanel
          domain={domain}
          serpChecks={scan.serpChecks}
          values={scan.values}
          onComplete={() => setGoogleDone(true)}
        />
      ) : null}
      {streamReady && step === 4 ? (
        <AiPanel
          domain={domain}
          geoPrompts={scan.geoPrompts}
          geoReplies={scan.geoReplies}
          values={scan.values}
          onComplete={() => setAiDone(true)}
        />
      ) : null}
      {streamReady && step === 5 ? (
        <CompetitorsPanel domain={domain} competitors={scan.competitors} values={scan.values} />
      ) : null}
      {streamReady && step === 6 ? (
        <ReadyPanel projectId={projectId} summary={summary} scan={scan} />
      ) : null}
    </ScanExperience>
  );
}

function buildSubtasks(
  scan: ReturnType<typeof useScanStream>,
  step: number,
  domain: string,
): { label: string; detail?: string; done?: boolean }[] {
  if (step === 1) {
    const crawled = scanNumber(scan.values, "pages_crawled") ?? scan.crawledPages.length;
    const brand = scanString(scan.values, "brand_name");
    return [
      {
        label: "Homepage",
        detail: crawled > 0 ? "open" : undefined,
        done: crawled > 0,
      },
      {
        label: crawled > 1 ? `${crawled - 1} more pages` : "Other pages",
        detail: crawled > 1 ? String(crawled - 1) : undefined,
        done: crawled > 1,
      },
      ...(brand ? [{ label: brand, detail: "recognized", done: true }] : []),
    ];
  }

  if (step === 2) {
    const avgMs = scanNumber(scan.values, "avg_response_ms");
    const https = scan.values.https;
    const sitemap = scan.values.sitemap;
    const robots = scan.values.robots_txt;
    const missingTitles = scanNumber(scan.values, "missing_titles");
    const canFind =
      typeof sitemap === "boolean" || typeof robots === "boolean"
        ? sitemap === true || robots === true
        : null;
    return [
      {
        label: "Shopper trust",
        detail: typeof https === "boolean" ? (https ? "Secure" : "At risk") : undefined,
        done: typeof https === "boolean",
      },
      {
        label: "Page feel",
        detail:
          avgMs == null
            ? undefined
            : avgMs < 1800
              ? "Snappy"
              : avgMs < 3000
                ? "Okay"
                : "Slow",
        done: avgMs != null,
      },
      {
        label: "Google can explore",
        detail: canFind == null ? undefined : canFind ? "Yes" : "Risky",
        done: canFind != null,
      },
      {
        label: "How listings look",
        detail:
          missingTitles != null
            ? missingTitles === 0
              ? "Clear"
              : `${missingTitles} weak`
            : undefined,
        done: missingTitles != null,
      },
    ];
  }

  if (step === 3) {
    return [{ label: "Searching Google like a buyer", done: false }];
  }

  if (step === 5) {
    const discovered = scanNumber(scan.values, "competitors_discovered") ?? scan.competitors.length;
    const crawled = scanNumber(scan.values, "competitors_crawled") ?? 0;
    const checking = (scan.values.domain as string | undefined)?.replace(/^www\./, "") ?? "";

    const items: { label: string; detail?: string; done?: boolean }[] = [
      {
        label: "Rivals from search",
        done: discovered > 0,
      },
    ];

    // Show up to 5 competitors with their crawl status
    const topComps = scan.competitors.slice(0, 8);
    for (const c of topComps) {
      const compDomain = c.domain.replace(/^www\./, "");
      const isClassified = Boolean(c.classification && c.classification !== "Classifying…" && c.classification !== "unknown");
      const isActive = Boolean(checking && compDomain === checking && !isClassified);
      items.push({
        label: compDomain,
        detail: isClassified ? "✓" : isActive ? "crawling…" : crawled > 0 ? "queued" : undefined,
        done: isClassified,
      });
    }

    // If currently checking a domain not in the top5 list yet, surface it
    if (checking && !topComps.some((c) => c.domain.replace(/^www\./, "") === checking)) {
      items.push({ label: checking, detail: "crawling…", done: false });
    }

    return items;
  }

  if (step === 4) {
    const replies = scan.geoReplies.filter((r) => r.response && !r.failed);
    const models = ["ChatGPT", "Claude", "Gemini", "Grok"] as const;
    const aliases = [["chatgpt", "gpt"], ["claude"], ["gemini"], ["grok", "xai", "perplexity", "sonar"]];
    return models.map((name, index) => {
      const done = replies.some((r) => aliases[index].some((a) => r.model.includes(a)));
      return {
        label: `Asking ${name}`,
        detail: done ? "✓" : undefined,
        done,
      };
    });
  }

  return [{ label: domain, done: true }];
}
