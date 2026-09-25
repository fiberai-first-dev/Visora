import type { ReactNode } from "react";

/**
 * Neo-brutalist primitives. Every page composes these so the visual language is
 * defined once: hard black borders, offset solid shadows, uppercase black type
 * and flat accents. Props are unchanged from the previous design on purpose, so
 * existing call sites keep working.
 */

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`b-card p-5 ${className}`}>{children}</div>;
}

export type BadgeTone =
  | "critical"
  | "warning"
  | "notice"
  | "good"
  | "neutral"
  | "info"
  | "yellow"
  | "dark"
  | string;

const badgeStyles: Record<string, string> = {
  critical: "b-badge-critical",
  warning: "b-badge-warning",
  notice: "b-badge-notice",
  info: "b-badge-notice",
  good: "b-badge-good",
  neutral: "b-badge-neutral",
  yellow: "b-badge-yellow",
  dark: "b-badge-dark",
};

export function Badge({ children, tone = "neutral" }: { children: ReactNode; tone?: BadgeTone }) {
  const style = badgeStyles[String(tone)] ?? badgeStyles.neutral;
  return <span className={`b-badge ${style}`}>{children}</span>;
}

export function severityTone(severity: string): BadgeTone {
  const normalized = (severity ?? "").toLowerCase();
  if (normalized === "critical" || normalized === "high") return "critical";
  if (normalized === "warning" || normalized === "medium") return "warning";
  if (normalized === "notice" || normalized === "info") return "notice";
  if (normalized === "good" || normalized === "low") return "good";
  return "neutral";
}

export function SectionHeader({ title, description }: { title: string; description?: string }) {
  return (
    <div className="mb-4">
      <h2 className="b-display text-xl">{title}</h2>
      {description ? <p className="mt-1 text-sm font-bold text-zinc-500">{description}</p> : null}
    </div>
  );
}

export function MetricCard({
  label,
  value,
  sub,
  tone = "neutral",
}: {
  label: string;
  value: string | number;
  sub?: string;
  tone?: "good" | "warn" | "bad" | "neutral";
}) {
  const valueTone: Record<string, string> = {
    good: "text-[var(--brutal-emerald)]",
    warn: "text-[var(--brutal-amber)]",
    bad: "text-[var(--brutal-rose)]",
    neutral: "text-black",
  };
  return (
    <div className="b-card flex flex-col gap-1 p-4">
      <span className="b-kicker text-zinc-500">{label}</span>
      <span className={`b-mono text-3xl font-black tabular-nums ${valueTone[tone]}`}>{value}</span>
      {sub ? <span className="text-xs font-bold text-zinc-500">{sub}</span> : null}
    </div>
  );
}

export function DataTable({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <div className={`b-card-flat w-full overflow-x-auto ${className}`}>
      <table className="b-table">{children}</table>
    </div>
  );
}

export function EmptyState({
  icon,
  title,
  hint,
  action,
}: {
  icon?: ReactNode;
  title: string;
  hint?: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 border-4 border-dashed border-zinc-400 bg-white p-8 text-center">
      {icon ? <div className="text-2xl">{icon}</div> : null}
      <div>
        <p className="b-display text-base">{title}</p>
        {hint ? <p className="mt-1 text-sm font-bold text-zinc-500">{hint}</p> : null}
      </div>
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}

export function Pill({ children, color = "b-badge-neutral" }: { children: ReactNode; color?: string }) {
  return <span className={`b-badge ${color}`}>{children}</span>;
}

export function Score({ value, label }: { value: number | null; label: string }) {
  const rounded = value === null || value === undefined ? null : Math.round(value);
  const tone =
    rounded === null
      ? "text-zinc-400"
      : rounded >= 70
        ? "text-[var(--brutal-emerald)]"
        : rounded >= 40
          ? "text-[var(--brutal-amber)]"
          : "text-[var(--brutal-rose)]";
  return (
    <div className="flex items-baseline gap-2">
      <span className={`b-mono text-5xl font-black tabular-nums ${tone}`}>{rounded ?? "–"}</span>
      <span className="b-kicker text-zinc-500">{label}</span>
    </div>
  );
}

export function MetricBar({
  label,
  value,
  hint,
}: {
  label: string;
  value: number | null; // 0..1
  hint?: string;
}) {
  const pct = value === null || value === undefined ? null : Math.round(value * 100);
  const color =
    pct === null
      ? "bg-zinc-300"
      : pct >= 60
        ? "bg-[var(--brutal-emerald)]"
        : pct >= 30
          ? "bg-[var(--brutal-amber)]"
          : "bg-[var(--brutal-rose)]";
  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <span className="b-kicker text-zinc-500">{label}</span>
        <span className="b-mono text-xs font-black tabular-nums">{pct === null ? "–" : `${pct}%`}</span>
      </div>
      <div
        className="h-4 w-full border-black bg-zinc-100"
        style={{ borderWidth: 3, borderStyle: "solid" }}
      >
        <div className={`h-full ${color} transition-all duration-500`} style={{ width: `${pct ?? 0}%` }} />
      </div>
      {hint ? <p className="mt-2 text-xs font-bold text-zinc-500">{hint}</p> : null}
    </div>
  );
}

export function StatRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div
      className="flex items-center justify-between py-3 text-sm"
      style={{ borderBottomWidth: 2, borderBottomStyle: "solid", borderColor: "#e4e4e7" }}
    >
      <span className="b-label text-zinc-500">{label}</span>
      <span className="font-bold">{value}</span>
    </div>
  );
}

// ── Backward-compat alias ─────────────────────────────────────────────────────
// Old pages imported SectionTitle; new name is SectionHeader.
export function SectionTitle({ children, hint }: { children: ReactNode; hint?: string }) {
  return (
    <SectionHeader
      title={typeof children === "string" ? children : String(children)}
      description={hint}
    />
  );
}
