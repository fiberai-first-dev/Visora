/** Scan journey — user outcomes, not internal pipeline jargon. */

export const JOURNEY_STEPS = [
  {
    id: 1,
    key: "health",
    label: "Your site",
    title: "Looking at your website",
    description: "Browsing your pages the way a visitor would — so we know what you actually offer.",
  },
  {
    id: 2,
    key: "technical",
    label: "Site quality",
    title: "Is your site ready for visitors?",
    description: "Trust, speed, and how you look when someone finds you on Google.",
  },
  {
    id: 3,
    key: "google",
    label: "Google",
    title: "Where do you show up on Google?",
    description: "Searching the way your buyers search — and seeing if you appear.",
  },
  {
    id: 4,
    key: "ai",
    label: "AI answers",
    title: "Do the AIs recommend you?",
    description: "Your buyers ask ChatGPT, Claude, Gemini and Grok before they open Google.",
  },
  {
    id: 5,
    key: "competitors",
    label: "Who else shows up",
    title: "Who shows up instead of you",
    description: "The other brands buyers see when they look for what you sell.",
  },
  {
    id: 6,
    key: "ready",
    label: "What to fix",
    title: "Here is what matters",
    description: "Clear next steps so more of the right people find you.",
  },
] as const;

export type JourneyStepId = (typeof JOURNEY_STEPS)[number]["id"];

/**
 * Map backend pipeline stages onto the friendly journey.
 * 1 crawl → your site (page screens)
 * 2 audit → site quality (speed / trust metrics)
 * 3 brand → still site quality (dwell) until Google intents start
 * 4–5 intents+SERP → google
 * 9–10 GEO → ai (runs right after Google)
 * 6–8 competitors → who else (runs after GEO)
 * 11+ / finished → what to fix
 */
export function journeyStepFromStage(stage: number, finished: boolean): JourneyStepId {
  if (finished || stage >= 11) return 6;
  if (stage >= 9) return 4;
  if (stage >= 6) return 5;
  if (stage >= 4) return 3;
  if (stage >= 2) return 2;
  if (stage >= 1) return 1;
  return 1;
}

export function journeyProgressPercent(
  step: JourneyStepId,
  finished: boolean,
  running: boolean,
): number {
  if (finished) return 100;
  const base = ((step - 1) / JOURNEY_STEPS.length) * 100;
  const within = running ? 100 / JOURNEY_STEPS.length / 2 : 0;
  return Math.min(99, Math.round(base + within));
}

export function estimateSecondsRemaining(step: JourneyStepId, finished: boolean): number {
  if (finished) return 0;
  // Honest-ish ETA: competitors should feel quick after Google.
  const weights = [20, 8, 24, 28, 20, 16];
  return weights.slice(step - 1).reduce((a, b) => a + b, 0);
}

export type SerpCheckSample = {
  query: string;
  brandPosition: number;
  foundInTop50: boolean;
  results: {
    position: number;
    domain: string;
    title: string;
    url?: string;
    snippet?: string;
    own: boolean;
  }[];
  queriesChecked: number;
  top10: number;
  top50: number;
  notFound: number;
};

export type GeoPromptSample = {
  prompt: string;
  index: number;
  total: number;
  models: string[];
};

export type GeoModelReply = {
  model: string;
  prompt: string;
  response: string;
  failed?: boolean;
  at: string;
};

export type CompetitorSample = {
  domain: string;
  brandName?: string;
  classification?: string;
  appearances?: number;
  bestPosition?: number;
};
