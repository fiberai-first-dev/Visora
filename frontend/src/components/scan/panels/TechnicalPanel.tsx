"use client";

import { useEffect, useRef, useState } from "react";
import { CrawledPage, scanNumber } from "@/lib/useScanStream";

function SpeedCompare({ ms }: { ms: number | null }) {
  const EXPECTED = 800;
  const INDUSTRY = 1500;
  const SLOW = 3000;
  const max = 3500;
  const yours = ms == null ? null : Math.min(ms, max);
  const bar = (value: number, color: string, label: string, emphasize?: boolean) => (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-[11px]">
        <span className={emphasize ? "font-semibold text-[#1a1a18]" : "text-zinc-500"}>{label}</span>
        <span className={`tabular-nums ${emphasize ? "font-semibold text-[#1a1a18]" : "text-zinc-400"}`}>
          {value}ms
        </span>
      </div>
      <div className="h-3 overflow-hidden rounded-full bg-zinc-100">
        <div
          className={`h-full rounded-full transition-all duration-1000 ease-out ${color}`}
          style={{ width: `${Math.max(6, (value / max) * 100)}%` }}
        />
      </div>
    </div>
  );

  return (
    <div className="space-y-3">
      {bar(EXPECTED, "bg-emerald-500", "Target · under 800ms")}
      {bar(INDUSTRY, "bg-zinc-300", "Typical · ~1.5s")}
      {yours != null ? (
        bar(
          yours,
          yours <= EXPECTED ? "bg-emerald-600" : yours <= INDUSTRY ? "bg-amber-400" : "bg-[#a65a3a]",
          "Your site",
          true,
        )
      ) : (
        <div className="space-y-1">
          <div className="flex items-center justify-between text-[11px] text-zinc-500">
            <span>Your site</span>
            <span className="ai-dot" />
          </div>
          <div className="h-3 animate-pulse rounded-full bg-zinc-100" />
        </div>
      )}
      {bar(SLOW, "bg-red-200", "Slow · 3s+")}
    </div>
  );
}

function Speedometer({
  ms,
  label,
  loading,
  onSettled,
}: {
  ms: number | null;
  label: string;
  loading?: boolean;
  onSettled?: () => void;
}) {
  const TARGET_MS = 800;
  const INDUSTRY_MS = 1500;
  const max = 3000;
  const MIN_SWEEP_MS = 4500;
  const resultReady = !loading && ms != null;
  const clamped = ms == null ? 0 : Math.min(Math.max(ms, 0), max);
  const settleAngle = -90 + (clamped / max) * 180;
  const score =
    ms == null ? null : ms <= TARGET_MS ? 92 : ms <= INDUSTRY_MS ? 68 : ms <= 2500 ? 42 : 22;
  const tone =
    ms == null ? "wait" : ms <= TARGET_MS ? "good" : ms <= INDUSTRY_MS ? "ok" : "slow";
  const color =
    tone === "good" ? "#3d7a5a" : tone === "ok" ? "#d4a017" : tone === "slow" ? "#a65a3a" : "#a1a1aa";

  const [phase, setPhase] = useState<"sweep" | "settle" | "done">("sweep");
  const [needle, setNeedle] = useState(-78);
  const needleRef = useRef(-78);
  const sweepStartedAt = useRef(typeof performance !== "undefined" ? performance.now() : 0);
  const onSettledRef = useRef(onSettled);
  onSettledRef.current = onSettled;

  useEffect(() => {
    if (phase === "done") return;

    if (phase === "sweep") {
      let raf = 0;
      const startT = sweepStartedAt.current;
      const tick = (now: number) => {
        const elapsed = now - startT;
        const pos = -78 * Math.cos((elapsed * Math.PI) / 1800);
        needleRef.current = pos;
        setNeedle(pos);
        const sweptLongEnough = now - sweepStartedAt.current >= MIN_SWEEP_MS;
        if (resultReady && sweptLongEnough) {
          setPhase("settle");
          return;
        }
        raf = window.requestAnimationFrame(tick);
      };
      raf = window.requestAnimationFrame(tick);
      return () => window.cancelAnimationFrame(raf);
    }

    const start = needleRef.current;
    const overshoot = settleAngle + (settleAngle >= start ? 10 : -10);
    const t0 = performance.now();
    const duration = 1200;
    let raf = 0;
    const tick = (now: number) => {
      const t = Math.min(1, (now - t0) / duration);
      let angle: number;
      if (t < 0.72) {
        const local = t / 0.72;
        const ease = 1 - Math.pow(1 - local, 3);
        angle = start + (overshoot - start) * ease;
      } else {
        const local = (t - 0.72) / 0.28;
        const ease = 1 - Math.pow(1 - local, 3);
        angle = overshoot + (settleAngle - overshoot) * ease;
      }
      needleRef.current = angle;
      setNeedle(angle);
      if (t < 1) {
        raf = window.requestAnimationFrame(tick);
        return;
      }
      needleRef.current = settleAngle;
      setNeedle(settleAngle);
      setPhase("done");
      onSettledRef.current?.();
    };
    raf = window.requestAnimationFrame(tick);
    return () => window.cancelAnimationFrame(raf);
  }, [phase, resultReady, settleAngle]);

  const searching = phase !== "done";

  return (
    <div className="relative mx-auto flex w-full max-w-md flex-col items-center">
      <svg viewBox="0 0 240 140" className="w-full max-w-[300px]" aria-hidden>
        <defs>
          <linearGradient id="siteQualityGauge" x1="0%" y1="0%" x2="100%" y2="0%">
            <stop offset="0%" stopColor="#3d7a5a" />
            <stop offset="45%" stopColor="#d4a017" />
            <stop offset="100%" stopColor="#a65a3a" />
          </linearGradient>
        </defs>
        <path
          d="M 20 120 A 100 100 0 0 1 220 120"
          fill="none"
          stroke="#e8e4dc"
          strokeWidth="16"
          strokeLinecap="round"
        />
        <path
          d="M 20 120 A 100 100 0 0 1 220 120"
          fill="none"
          stroke="url(#siteQualityGauge)"
          strokeWidth="16"
          strokeLinecap="round"
          opacity={searching ? 0.45 : 0.9}
        />
        {(() => {
          const a = ((-90 + (TARGET_MS / max) * 180) * Math.PI) / 180;
          const x1 = 120 + Math.cos(a) * 78;
          const y1 = 120 + Math.sin(a) * 78;
          const x2 = 120 + Math.cos(a) * 100;
          const y2 = 120 + Math.sin(a) * 100;
          return <line x1={x1} y1={y1} x2={x2} y2={y2} stroke="#1a1a18" strokeWidth="2.5" />;
        })()}
        <g transform={`rotate(${needle} 120 120)`}>
          <line
            x1="120"
            y1="120"
            x2="120"
            y2="38"
            stroke={searching ? "#71717a" : color}
            strokeWidth="3.5"
            strokeLinecap="round"
          />
          <circle cx="120" cy="120" r="7" fill={searching ? "#71717a" : color} />
          <circle cx="120" cy="120" r="3" fill="#fff" />
        </g>
        <text x="28" y="136" fontSize="10" fill="#a1a1aa">
          Fast
        </text>
        <text x="198" y="136" fontSize="10" fill="#a1a1aa">
          Slow
        </text>
      </svg>
      <div className="mt-1 text-center">
        {searching ? (
          <>
            <div className="mx-auto flex h-10 items-center justify-center gap-1.5">
              <span className="ai-dot" />
              <span className="ai-dot" style={{ animationDelay: "0.15s" }} />
              <span className="ai-dot" style={{ animationDelay: "0.3s" }} />
            </div>
            <p className="mt-1 text-sm font-semibold text-zinc-400">Timing pages…</p>
          </>
        ) : (
          <>
            <p className="font-[family-name:var(--font-display)] text-4xl tabular-nums text-[#1a1a18]">
              {Math.round(ms!)}
              <span className="ml-1 text-lg font-sans font-semibold text-zinc-400">ms</span>
            </p>
            <p className="mt-1 text-sm font-semibold" style={{ color }}>
              {label}
            </p>
            {score != null ? (
              <p className="mt-0.5 text-[11px] font-medium uppercase tracking-wide text-zinc-400">
                {score}/100
              </p>
            ) : null}
          </>
        )}
      </div>
    </div>
  );
}

export function TechnicalPanel({
  pages,
  values,
}: {
  domain?: string;
  pages?: CrawledPage[];
  values: Record<string, unknown>;
}) {
  const avgMs = scanNumber(values, "avg_response_ms");
  const ready = avgMs != null || (pages?.length ?? 0) > 0;
  const [gaugeRevealed, setGaugeRevealed] = useState(false);

  return (
    <div className="mx-auto flex h-full w-full max-w-xl flex-col items-center justify-center px-3">
      <div className="w-full rounded-[28px] bg-white px-6 py-8 shadow-sm ring-1 ring-black/5 sm:px-10 sm:py-10">
        <p className="text-center text-[11px] font-semibold uppercase tracking-[0.18em] text-zinc-400">
          Page speed
        </p>
        <div className="mt-4">
          <Speedometer
            ms={avgMs}
            label={
              avgMs == null ? "Timing…" : avgMs <= 800 ? "Good" : avgMs <= 1500 ? "OK" : "Slow"
            }
            loading={!ready || avgMs == null}
            onSettled={() => setGaugeRevealed(true)}
          />
        </div>
        <div className="mt-8 border-t border-zinc-100 pt-6">
          <SpeedCompare ms={gaugeRevealed ? avgMs : null} />
        </div>
      </div>
    </div>
  );
}
