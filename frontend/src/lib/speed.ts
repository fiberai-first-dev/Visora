/** Shared crawl-speed verdict used on Site health + Overview. */
export const SPEED_TARGET_MS = 800;
export const SPEED_INDUSTRY_MS = 1500;

export type SpeedVerdict = {
  label: string;
  tone: "good" | "ok" | "bad" | "unknown";
};

export function speedVerdict(ms: number | null | undefined): SpeedVerdict {
  if (ms == null || ms <= 0) return { label: "Not measured", tone: "unknown" };
  if (ms <= SPEED_TARGET_MS) return { label: "Good", tone: "good" };
  if (ms <= SPEED_INDUSTRY_MS) return { label: "OK", tone: "ok" };
  if (ms <= 2500) return { label: "Slow", tone: "bad" };
  return { label: "Poor", tone: "bad" };
}
