/** Turn a crawl dump into something a person can read. */

function collapse(text?: string | null): string {
  return (text || "").replace(/\s+/g, " ").trim();
}

export function readableCopy(text?: string | null, max = 240): string {
  const clean = collapse(text);
  if (!clean) return "";
  if (clean.length <= max) return clean;

  if ((text || "").includes("\n")) {
    const lines = (text || "")
      .split("\n")
      .map((line) => collapse(line))
      .filter((line) => line.split(" ").length >= 6);
    let out = "";
    for (const line of lines) {
      const next = out ? `${out}\n${line}` : line;
      if (next.length > max && out) break;
      out = next;
    }
    if (out) return out.length > max ? `${out.slice(0, max)}…` : out;
  }

  const sentences = clean.match(/[^.!?]+[.!?]+/g);
  if (sentences && sentences.length > 0) {
    let out = "";
    for (const raw of sentences.slice(0, 2)) {
      const bit = raw.trim();
      if (bit.split(" ").length < 4) continue;
      const next = out ? `${out} ${bit}` : bit;
      if (next.length > max && out) break;
      out = next;
    }
    if (out) return out.length > max ? `${out.slice(0, max)}…` : out;
  }

  const words = clean.split(" ");
  if (words.length > 22) return "";
  return clean.length > max ? `${clean.slice(0, max)}…` : clean;
}

export function pasteReadyAfter(text?: string | null): string {
  let clean = (text || "").trim();
  if (!clean) return "";
  clean = clean
    .replace(/Add a section on this page[^.!?]*[.!?]?/gi, "")
    .replace(/Create or strengthen a page[^.!?]*[.!?]?/gi, "")
    .replace(/currently outranks you[^.!?]*[.!?]?/gi, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
  return clean || collapse(text);
}
