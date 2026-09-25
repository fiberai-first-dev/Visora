"use client";

import { useEffect } from "react";
import {
  JOURNEY_STEPS,
  JourneyStepId,
  estimateSecondsRemaining,
  journeyProgressPercent,
} from "@/lib/journey";

type Subtask = { label: string; detail?: string; done?: boolean };

export function ScanExperience({
  domain,
  step,
  finished,
  running,
  stopped,
  pauseDetail,
  onRestart,
  restarting,
  subtasks,
  children,
}: {
  domain: string;
  step: JourneyStepId;
  finished: boolean;
  running: boolean;
  stopped: boolean;
  pauseDetail?: string;
  onRestart?: () => void;
  restarting?: boolean;
  subtasks: Subtask[];
  children: React.ReactNode;
}) {
  const progress = journeyProgressPercent(step, finished, running);
  const eta = estimateSecondsRemaining(step, finished);

  // Lock document scroll for the whole scan — only inner panels may scroll.
  useEffect(() => {
    const prevHtml = document.documentElement.style.overflow;
    const prevBody = document.body.style.overflow;
    document.documentElement.style.overflow = "hidden";
    document.body.style.overflow = "hidden";
    return () => {
      document.documentElement.style.overflow = prevHtml;
      document.body.style.overflow = prevBody;
    };
  }, []);

  return (
    <div className="scan-app flex h-dvh max-h-dvh flex-col overflow-hidden">
      <header className="relative z-20 flex shrink-0 items-center justify-between border-b border-black/[0.04] bg-[#f7f3ec]/80 px-5 py-3 backdrop-blur-md md:px-8">
        <div className="flex items-center gap-3">
          <div className="min-w-0">
            <div className="truncate text-[10px] font-medium uppercase tracking-[0.14em] text-zinc-400">
              {domain}
            </div>
          </div>
        </div>
        <span
          className={`rounded-full px-3 py-1 text-[11px] font-semibold uppercase tracking-wide ${
            finished
              ? "bg-emerald-100 text-emerald-800"
              : stopped
                ? "bg-amber-100 text-amber-800"
                : "bg-white/80 text-zinc-600 ring-1 ring-black/5"
          }`}
        >
          {finished ? "Ready" : stopped ? "Paused" : running ? "Live" : "Starting"}
        </span>
      </header>

      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden p-3 md:flex-row md:gap-0 md:p-4 lg:p-5">
        {/* Fixed-width sidebar — never grows with content */}
        <aside className="scan-sidebar relative flex h-auto max-h-[38vh] w-full shrink-0 flex-col overflow-hidden rounded-2xl text-white md:h-full md:max-h-none md:w-[280px] lg:w-[300px]">
          <div className="scan-sidebar-glow pointer-events-none absolute inset-0" />
          <div className="relative z-10 flex min-h-0 flex-1 flex-col p-4 md:p-5">
            <nav
              className="scan-sidebar-nav min-h-0 flex-1 overflow-y-auto overscroll-contain pr-1"
              aria-label="Scan progress"
            >
              <div className="flex flex-col gap-0.5">
                {JOURNEY_STEPS.map((item) => {
                  const done = item.id < step;
                  const current = item.id === step;
                  return (
                    <div
                      key={item.key}
                      className={`rounded-xl px-3 py-2 transition-colors duration-300 ${
                        current ? "bg-white/[0.12] ring-1 ring-white/20" : ""
                      }`}
                    >
                      <div className="flex items-center gap-3">
                        <StatusIcon done={done} current={current} />
                        <span
                          className={`text-sm font-medium transition-colors ${
                            current ? "text-white" : done ? "text-white/75" : "text-white/30"
                          }`}
                        >
                          {item.label}
                        </span>
                      </div>

                      {current && subtasks.length > 0 ? (
                        <ul className="mt-2 space-y-1.5 border-l border-white/10 pl-5 ml-2">
                          {subtasks.map((task) => (
                            <li
                              key={task.label}
                              className="flex items-center justify-between gap-2 text-xs text-white/55"
                            >
                              <span className="flex min-w-0 items-center gap-2 truncate">
                                <span
                                  className={`h-1.5 w-1.5 shrink-0 rounded-full ${
                                    task.done
                                      ? "bg-emerald-400"
                                      : task.detail === "crawling…"
                                        ? "bg-amber-400 scan-pulse-dot"
                                        : "scan-pulse-dot bg-white/50"
                                  }`}
                                />
                                <span className={`truncate ${task.detail === "crawling…" ? "text-white/80 font-medium" : ""}`}>
                                  {task.label}
                                </span>
                              </span>
                              {task.detail ? (
                                <span
                                  className={`shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold ${
                                    task.detail === "✓"
                                      ? "bg-emerald-500/20 text-emerald-400"
                                      : task.detail === "crawling…"
                                        ? "bg-amber-500/20 text-amber-400"
                                        : "bg-white/10 text-white/80"
                                  }`}
                                >
                                  {task.detail}
                                </span>
                              ) : null}
                            </li>
                          ))}
                        </ul>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            </nav>

            <div className="mt-4 shrink-0">
              <div className="mb-2 flex items-center justify-between text-xs text-white/45">
                <span>
                  {finished ? "All set" : stopped ? "Paused" : `About ${eta}s left`}
                </span>
                <span className="font-semibold tabular-nums text-white/70">{progress}%</span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-white/10">
                <div
                  className="scan-progress-bar h-full rounded-full bg-gradient-to-r from-white/90 to-white"
                  style={{ width: `${progress}%` }}
                />
              </div>
            </div>
          </div>
        </aside>

        <main className="scan-main relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-2xl md:ml-4">
          <div className="scan-main-sheen pointer-events-none absolute inset-0" />
          <div className="relative z-10 flex min-h-0 flex-1 flex-col overflow-hidden p-3 md:p-4">
            {stopped ? (
              <div className="mb-4 shrink-0 rounded-2xl border border-red-200/80 bg-red-50/90 px-4 py-3 text-sm text-[#1a1a18] shadow-sm">
                <p className="font-semibold">Something interrupted the scan.</p>
                <p className="mt-1 text-zinc-600">{pauseDetail}</p>
                {onRestart ? (
                  <button
                    type="button"
                    onClick={onRestart}
                    disabled={restarting}
                    className="mt-3 rounded-xl bg-[#1a1a18] px-4 py-2 text-xs font-semibold uppercase tracking-wide text-white transition hover:bg-[#a65a3a] disabled:opacity-50"
                  >
                    {restarting ? "Restarting…" : "Restart scan"}
                  </button>
                ) : null}
              </div>
            ) : null}

          <div
              key={`panel-${step}`}
              className="scan-step-enter flex min-h-0 flex-1 flex-col overflow-y-auto overflow-x-hidden"
            >
              {children}
            </div>
          </div>
        </main>
      </div>
    </div>
  );
}

function StatusIcon({ done, current }: { done: boolean; current: boolean }) {
  if (done && !current) {
    return (
      <span className="flex h-5 w-5 items-center justify-center rounded-full bg-white text-[10px] font-black text-[#1a1a18]">
        ✓
      </span>
    );
  }
  if (current) {
    return (
      <span className="relative flex h-5 w-5 items-center justify-center">
        <span className="absolute inset-0 animate-ping rounded-full bg-white/25" />
        <span className="h-2.5 w-2.5 rounded-full bg-white shadow-[0_0_12px_rgba(255,255,255,0.6)]" />
      </span>
    );
  }
  return <span className="h-5 w-5 rounded-full border border-white/20" />;
}
