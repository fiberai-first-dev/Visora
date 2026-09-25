"use client";

import React, { useEffect, useMemo, useRef, useState } from "react";
import { GeoModelReply, GeoPromptSample } from "@/lib/journey";

type ModelId = "chatgpt" | "claude" | "gemini" | "grok";

const MODELS: {
  id: ModelId;
  name: string;
  product: string;
  aliases: string[];
}[] = [
  { id: "chatgpt", name: "ChatGPT", product: "OpenAI · GPT-4o", aliases: ["chatgpt", "gpt-4o", "gpt"] },
  { id: "claude", name: "Claude", product: "Anthropic · claude-3-5-sonnet", aliases: ["claude"] },
  { id: "gemini", name: "Gemini", product: "Google · Gemini 2.0", aliases: ["gemini"] },
  { id: "grok", name: "Grok", product: "xAI · Grok-2", aliases: ["grok", "xai", "perplexity", "sonar"] },
];

/** Type like a person: char-by-char for short text, slightly faster for longer. */
function useTypewriter(text: string, active: boolean, cps = 42) {
  const [shown, setShown] = useState("");
  const [done, setDone] = useState(false);

  useEffect(() => {
    setShown("");
    setDone(false);
    if (!active || !text) {
      if (!text) setDone(true);
      return;
    }
    let i = 0;
    const tickMs = Math.max(12, Math.round(1000 / cps));
    const id = window.setInterval(() => {
      i += 1;
      setShown(text.slice(0, i));
      if (i >= text.length) {
        setDone(true);
        window.clearInterval(id);
      }
    }, tickMs);
    return () => window.clearInterval(id);
  }, [text, active, cps]);

  return { shown, done };
}

function highlightBrand(text: string, domain: string): React.ReactNode {
  const clean = stripInlineMarkdown(text);
  if (!domain || !clean) return clean;
  const brand = domain.replace(/^www\./, "").split(".")[0];
  if (!brand || brand.length < 2) return clean;
  const escaped = brand.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const re = new RegExp(`(${escaped})`, "gi");
  const parts = clean.split(re);
  return parts.map((part, i) =>
    part.toLowerCase() === brand.toLowerCase() ? (
      <mark key={i} className="rounded bg-amber-200/80 px-0.5 text-inherit">
        {part}
      </mark>
    ) : (
      <span key={i}>{part}</span>
    ),
  );
}

/** Kill leftover markdown so chats never show raw * / ** / ####. */
function stripInlineMarkdown(s: string): string {
  return s
    .replace(/\*\*([^*]+)\*\*/g, "$1")
    .replace(/__([^_]+)__/g, "$1")
    .replace(/(^|[^*\w])\*([^*\n]+)\*([^*\w]|$)/g, "$1$2$3")
    .replace(/`+/g, "")
    .replace(/\*/g, "")
    .replace(/^#{1,6}\s+/gm, "")
    .trim();
}

function cleanAnswer(text: string): string {
  return text
    .split("\n")
    .map((line) => {
      let s = line.trim();
      s = s.replace(/^#{1,6}\s+/, "");
      s = s.replace(/^(\*\s+|\*\*\s+|•\s+)/, "- ");
      s = stripInlineMarkdown(s);
      return s;
    })
    .filter((s, i, arr) => !(s === "" && (i === 0 || arr[i - 1] === "")))
    .join("\n")
    .trim();
}

function formatAnswer(text: string, domain: string): React.ReactNode {
  const lines = cleanAnswer(text).split("\n");
  return lines.map((line, li) => {
    const trimmed = line.trim();
    const bullet = trimmed.match(/^[-•]\s+(.*)$/);
    const numbered = trimmed.match(/^\d+[.)]\s+(.*)$/);
    if (bullet || numbered) {
      const body = (bullet?.[1] ?? numbered?.[1] ?? "").trim();
      const mark = numbered ? `${trimmed.match(/^(\d+)/)?.[1]}.` : "•";
      return (
        <div key={li} className="flex gap-2 py-0.5">
          <span className="w-4 shrink-0 text-right opacity-50">{mark}</span>
          <span className="min-w-0">{highlightBrand(body, domain)}</span>
        </div>
      );
    }
    if (!trimmed) return <div key={li} className="h-2" />;
    return (
      <p key={li} className="py-0.5">
        {highlightBrand(trimmed, domain)}
      </p>
    );
  });
}

function ModelLogo({ id, size = "md" }: { id: ModelId; size?: "sm" | "md" | "lg" }) {
  const sizeMap = {
    sm: { box: "h-6 w-6", icon: "h-3.5 w-3.5" },
    md: { box: "h-9 w-9", icon: "h-5 w-5" },
    lg: { box: "h-10 w-10", icon: "h-6 w-6" },
  };
  const { box, icon } = sizeMap[size];

  if (id === "chatgpt") {
    return (
      <span className={`flex ${box} items-center justify-center rounded-full bg-[#10a37f] text-white`}>
        <svg viewBox="0 0 24 24" className={icon} fill="currentColor" aria-hidden>
          <path d="M22.282 9.821a5.985 5.985 0 0 0-.516-4.91 6.046 6.046 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a5.985 5.985 0 0 0-3.998 2.9 6.046 6.046 0 0 0 .743 7.097 5.98 5.98 0 0 0 .51 4.911 6.051 6.051 0 0 0 6.515 2.9A5.985 5.985 0 0 0 13.26 24a6.056 6.056 0 0 0 5.772-4.206 5.985 5.985 0 0 0 3.997-2.9 6.056 6.056 0 0 0-.747-7.073zM13.26 22.43a4.476 4.476 0 0 1-2.863-1.036l.141-.081 4.779-2.758a.795.795 0 0 0 .392-.681v-6.737l2.02 1.168a.071.071 0 0 1 .038.052v5.583a4.504 4.504 0 0 1-4.507 4.49zM3.6 18.304a4.47 4.47 0 0 1-.535-3.014l.142.085 4.783 2.759a.771.771 0 0 0 .78 0l5.843-3.369v2.332a.08.08 0 0 1-.033.062L9.74 19.95a4.5 4.5 0 0 1-6.14-1.646zM2.34 7.896a4.485 4.485 0 0 1 2.366-1.973V11.6a.766.766 0 0 0 .388.676l5.815 3.355-2.02 1.168a.076.076 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872zm16.597 3.855l-5.833-3.387L15.124 7.2a.076.076 0 0 1 .071 0l4.83 2.787a4.471 4.471 0 0 1 .679 6.374l-.141-.085-4.778-2.758a.776.776 0 0 0-.79 0l-5.815 3.354v-2.332a.07.07 0 0 1 .032-.061l4.827-2.784a.78.78 0 0 0 .78 0l5.815-3.352zM8.97 5.577l-.142.08-4.779 2.762A.776.776 0 0 0 3.66 9.1v5.583a.07.07 0 0 1-.032.061L1.61 13.576a4.5 4.5 0 0 1 1.616-7.96 4.462 4.462 0 0 1 3.39.56l.141.08 4.779 2.758a.795.795 0 0 0 .392.681v6.737l-2.02-1.168a.071.071 0 0 1-.038-.052V8.98a.766.766 0 0 0-.388-.676L8.97 5.577z" />
        </svg>
      </span>
    );
  }
  if (id === "claude") {
    return (
      <span className={`flex ${box} items-center justify-center rounded-xl bg-[#d97757] text-white`}>
        <svg viewBox="0 0 24 24" className={icon} fill="currentColor" aria-hidden>
          <path d="M12.5 3.5c.4 1.8 1.1 3.4 2.1 4.9.9 1.4 2 2.6 3.4 3.5-1.4.9-2.5 2.1-3.4 3.5-.9 1.4-1.6 3-2 4.7-.4-1.7-1.1-3.3-2-4.7-.9-1.4-2-2.6-3.4-3.5 1.4-.9 2.5-2.1 3.4-3.5 1-1.5 1.7-3.1 2.1-4.9z" />
        </svg>
      </span>
    );
  }
  if (id === "gemini") {
    return (
      <span className={`flex ${box} items-center justify-center rounded-full bg-white shadow-sm ring-1 ring-black/5`}>
        <svg viewBox="0 0 24 24" className={icon} aria-hidden>
          <defs>
            <linearGradient id="gemGrad" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stopColor="#4285f4" />
              <stop offset="35%" stopColor="#9b72cb" />
              <stop offset="70%" stopColor="#d96570" />
              <stop offset="100%" stopColor="#d96570" />
            </linearGradient>
          </defs>
          <path fill="url(#gemGrad)" d="M12 2l1.5 7.5L21 11l-7.5 1.5L12 20l-1.5-7.5L3 11l7.5-1.5L12 2z" />
        </svg>
      </span>
    );
  }
  // Grok / xAI
  return (
    <span className={`flex ${box} items-center justify-center rounded-full bg-white text-black shadow-sm ring-1 ring-black/5`}>
      <svg viewBox="0 0 24 24" className={icon} fill="currentColor" aria-hidden>
        <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
      </svg>
    </span>
  );
}

function ChatShell({
  model,
  domain,
  prompt,
  reply,
  waiting,
}: {
  model: (typeof MODELS)[number];
  domain: string;
  prompt: string;
  reply?: GeoModelReply;
  waiting: boolean;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const responseText = reply?.failed
    ? "Couldn't answer right now."
    : reply?.response
      ? reply.response.slice(0, 900)
      : "";

  const { shown: typedAsk, done: askDone } = useTypewriter(prompt, Boolean(prompt), 55);
  const [answerReady, setAnswerReady] = useState(false);
  useEffect(() => {
    setAnswerReady(false);
    if (!askDone || !responseText) return;
    const t = window.setTimeout(() => setAnswerReady(true), 280);
    return () => window.clearTimeout(t);
  }, [askDone, responseText, prompt]);

  const { shown: typedAnswer, done: answerDone } = useTypewriter(
    responseText,
    answerReady,
    responseText.length > 400 ? 70 : 48,
  );
  const streamingAsk = Boolean(prompt) && !askDone;
  const streamingAnswer = answerReady && Boolean(responseText) && !answerDone;
  const showThinking = askDone && waiting && !responseText;
  const hasAnswer = Boolean(responseText) && answerReady;

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [typedAsk, typedAnswer, showThinking]);

  const status =
    reply?.failed
      ? "Failed"
      : hasAnswer && answerDone
        ? "Answered"
        : streamingAnswer || streamingAsk
          ? streamingAsk
            ? "Typing"
            : "Answering"
          : showThinking || waiting
            ? "Thinking"
            : "";

  const theme = shellTheme(model.id);

  return (
    <div
      className={`scan-card-appear flex h-full min-h-[240px] flex-col overflow-hidden shadow-lg ring-1 ${theme.shell} ${theme.radius}`}
    >
      {/* Header bar */}
      <div className={`flex items-center gap-2.5 border-b px-3.5 py-2.5 ${theme.header}`}>
        <ModelLogo id={model.id} />
        <div className="min-w-0 flex-1">
          <p className={`truncate text-[14px] font-semibold tracking-tight ${theme.title}`}>{model.name}</p>
        </div>
        <span className={`rounded-full px-2.5 py-1 text-[9px] font-bold uppercase tracking-wider ${statusClass(status, model.id)}`}>
          {status || "\u00a0"}
        </span>
      </div>

      {/* Chat area */}
      <div
        ref={scrollRef}
        className={`ai-chat-scroll flex-1 space-y-4 overflow-y-auto overscroll-contain px-3.5 py-4 ${theme.scroll} ${
          model.id === "chatgpt" || model.id === "grok" ? "ai-chat-scroll-dark" : "ai-chat-scroll-light"
        }`}
      >
        {prompt ? (
          <div className={theme.userBubble}>
            {typedAsk}
            {streamingAsk ? <span className="type-caret ml-0.5 inline-block" /> : null}
          </div>
        ) : (
          <p className={`pt-10 text-center text-xs ${theme.muted}`}>Waiting for buyer question…</p>
        )}

        {showThinking ? (
          <div className="flex items-start gap-2.5">
            <ModelLogo id={model.id} size="sm" />
            <div className={`flex items-center gap-1.5 pt-1.5 ${theme.thinkingWrap}`}>
              <span className="ai-dot" />
              <span className="ai-dot" style={{ animationDelay: "0.15s" }} />
              <span className="ai-dot" style={{ animationDelay: "0.3s" }} />
            </div>
          </div>
        ) : null}

        {hasAnswer ? (
          <div className={`flex gap-2.5 ${theme.assistantWrap}`}>
            <div className="mt-0.5 shrink-0">
              <ModelLogo id={model.id} size="sm" />
            </div>
            <div className={`min-w-0 flex-1 ${theme.assistant}`}>
              {formatAnswer(typedAnswer, domain)}
              {streamingAnswer ? <span className="type-caret ml-0.5 inline-block" /> : null}
            </div>
          </div>
        ) : null}
      </div>

      {/* Composer bar */}
      <div className={`border-t px-3 py-2.5 ${theme.footer}`}>
        <div className={`flex items-center gap-2 rounded-xl px-3.5 py-2.5 text-[12px] ${theme.composer}`}>
          <span className="min-w-0 flex-1 truncate">{theme.placeholder}</span>
          <span className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-[12px] font-bold ${theme.send}`}>
            {model.id === "gemini" ? "✦" : "↑"}
          </span>
        </div>
      </div>
    </div>
  );
}

function shellTheme(id: ModelId) {
  if (id === "chatgpt") {
    return {
      shell: "bg-[#212121] text-[#ececec] ring-black/40",
      radius: "rounded-2xl",
      header: "border-white/[0.08] bg-[#171717]",
      title: "text-white",
      sub: "text-white/40",
      scroll: "",
      thinkingWrap: "",
      userBubble: "ml-auto max-w-[88%] rounded-[1.4rem] bg-[#2f2f2f] px-4 py-2.5 text-[13.5px] leading-relaxed text-[#ececec]",
      assistantWrap: "",
      assistant: "text-[13.5px] leading-[1.65] text-[#ececec]",
      muted: "text-white/35",
      footer: "border-white/[0.08] bg-[#212121]",
      composer: "border border-white/10 bg-[#2f2f2f] text-white/35",
      send: "bg-white text-black",
      placeholder: "Message ChatGPT…",
    };
  }
  if (id === "claude") {
    return {
      shell: "bg-[#faf9f5] text-[#1f1e1d] ring-[#e5e0d5]",
      radius: "rounded-2xl",
      header: "border-[#ebe6dc] bg-[#f5f0e8]",
      title: "text-[#1f1e1d]",
      sub: "text-[#8a8276]",
      scroll: "",
      thinkingWrap: "",
      userBubble:
        "ml-auto max-w-[88%] rounded-2xl bg-[#ebe5da] px-4 py-2.5 text-[13.5px] leading-relaxed text-[#1f1e1d]",
      assistantWrap: "",
      assistant: "font-serif text-[14.5px] leading-[1.7] text-[#2c2a26]",
      muted: "text-[#a39c90]",
      footer: "border-[#ebe6dc] bg-[#faf9f5]",
      composer: "border border-[#ddd5c7] bg-white text-[#a39c90] shadow-sm",
      send: "bg-[#1f1e1d] text-white",
      placeholder: "Reply to Claude…",
    };
  }
  if (id === "gemini") {
    return {
      shell: "bg-[#f8f9fa] text-[#1f1f1f] ring-zinc-200/80",
      radius: "rounded-3xl",
      header: "border-zinc-200/80 bg-white",
      title: "bg-gradient-to-r from-[#4285f4] via-[#9b72cb] to-[#d96570] bg-clip-text text-transparent",
      sub: "text-zinc-500",
      scroll: "bg-[#f8f9fa]",
      thinkingWrap: "",
      userBubble:
        "ml-auto max-w-[88%] rounded-3xl bg-[#d3e3fd] px-4 py-2.5 text-[13.5px] leading-relaxed text-[#0842a0]",
      assistantWrap: "rounded-2xl bg-white p-3 shadow-sm ring-1 ring-black/5",
      assistant: "text-[13.5px] leading-[1.65] text-[#3c4043]",
      muted: "text-zinc-400",
      footer: "border-zinc-200/80 bg-white",
      composer: "border border-zinc-200 bg-[#f1f3f4] text-zinc-400",
      send: "bg-gradient-to-br from-[#4285f4] to-[#9b72cb] text-white",
      placeholder: "Enter a prompt for Gemini…",
    };
  }
  // Grok
  return {
    shell: "bg-black text-[#f4f4f5] ring-white/10",
    radius: "rounded-2xl",
    header: "border-white/10 bg-[#0a0a0a]",
    title: "text-white",
    sub: "text-white/40",
    scroll: "",
    thinkingWrap: "",
    userBubble: "ml-auto max-w-[88%] rounded-2xl bg-[#181818] px-4 py-2.5 text-[13.5px] leading-relaxed ring-1 ring-white/10",
    assistantWrap: "",
    assistant: "text-[13.5px] leading-[1.65] text-[#e4e4e7]",
    muted: "text-white/30",
    footer: "border-white/10 bg-black",
    composer: "border border-white/15 bg-[#111] text-white/35",
    send: "bg-white text-black",
    placeholder: "Ask Grok anything…",
  };
}

function statusClass(status: string, id: ModelId) {
  const dark = id === "chatgpt" || id === "grok";
  if (status === "Answered") return "bg-emerald-500/20 text-emerald-400";
  if (status === "Failed") return "bg-amber-500/20 text-amber-400";
  if (!status) return "bg-transparent text-transparent";
  return dark ? "bg-white/10 text-white/55" : "bg-zinc-200/80 text-zinc-500";
}

const READ_AFTER_ANSWERS_MS = 9000;
const MAX_PER_QUESTION_MS = 18000;

export function AiPanel({
  domain,
  geoPrompts,
  geoReplies,
  onComplete,
}: {
  domain: string;
  geoPrompts: GeoPromptSample[];
  geoReplies: GeoModelReply[];
  values?: Record<string, unknown>;
  onComplete?: () => void;
}) {
  const allPrompts = useMemo(() => {
    const unique: string[] = [];
    for (const p of geoPrompts.map((row) => row.prompt?.trim()).filter(Boolean) as string[]) {
      if (!unique.some((u) => u.toLowerCase() === p.toLowerCase())) unique.push(p);
    }
    return unique.slice(0, 4);
  }, [geoPrompts]);

  const [promptIdx, setPromptIdx] = useState(0);
  const onCompleteRef = useRef(onComplete);
  onCompleteRef.current = onComplete;
  const completedRef = useRef(false);

  const currentPromptText = allPrompts[promptIdx] ?? "";

  const repliesByModel = useMemo(() => {
    const map = new Map<ModelId, GeoModelReply>();
    for (const model of MODELS) {
      const pick = geoReplies
        .filter((r) => r.prompt === currentPromptText && model.aliases.some((a) => r.model.includes(a)))
        .at(-1);
      if (pick) map.set(model.id, pick);
    }
    return map;
  }, [geoReplies, currentPromptText]);

  const allSettled =
    Boolean(currentPromptText) &&
    MODELS.every((m) => {
      const r = repliesByModel.get(m.id);
      return Boolean(r && (r.response || r.failed));
    });

  // Each question plays once, in order. Never loops back to an earlier one.
  const advance = React.useCallback(() => {
    if (promptIdx + 1 < allPrompts.length) {
      setPromptIdx(promptIdx + 1);
    } else if (!completedRef.current && allPrompts.length > 0) {
      completedRef.current = true;
      onCompleteRef.current?.();
    }
  }, [promptIdx, allPrompts.length]);

  useEffect(() => {
    if (!currentPromptText) return;
    const id = window.setTimeout(advance, MAX_PER_QUESTION_MS);
    return () => window.clearTimeout(id);
  }, [currentPromptText, advance]);

  useEffect(() => {
    if (!allSettled) return;
    const id = window.setTimeout(advance, READ_AFTER_ANSWERS_MS);
    return () => window.clearTimeout(id);
  }, [allSettled, advance]);

  return (
    <div className="mx-auto flex h-full min-h-0 w-full max-w-6xl flex-col gap-2">

      <div className="grid min-h-0 flex-1 grid-cols-1 grid-rows-none gap-2 overflow-y-auto overscroll-contain sm:grid-cols-2 sm:grid-rows-2 sm:overflow-hidden">
        {MODELS.map((model) => {
          const reply = repliesByModel.get(model.id);
          const settled = Boolean(reply?.response || reply?.failed);
          const waiting = !settled;
          return (
            <ChatShell
              key={`${model.id}-${currentPromptText}`}
              model={model}
              domain={domain}
              prompt={currentPromptText}
              reply={reply}
              waiting={waiting}
            />
          );
        })}
      </div>
    </div>
  );
}
