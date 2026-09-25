export const AUTH_COOKIE = "visora_token";

function expireDomains(hostname: string): (string | undefined)[] {
  const hosts = new Set<string | undefined>([undefined]);
  const add = (raw?: string) => {
    const d = (raw || "").replace(/^\./, "").trim().toLowerCase();
    if (!d || d === "localhost" || /^[\d.]+$/.test(d)) return;
    hosts.add(d);
    hosts.add(`.${d}`);
  };
  add(process.env.COOKIE_DOMAIN);
  const parts = hostname.split(".").filter(Boolean);
  for (let i = 0; i < parts.length - 1; i++) {
    add(parts.slice(i).join("."));
  }
  return [...hosts];
}

/** The host the browser actually used — behind nginx the request URL is the container's. */
export function publicHost(headers: Headers): string {
  const raw = headers.get("x-forwarded-host") || headers.get("host") || "";
  return raw.split(",")[0].trim().replace(/:\d+$/, "");
}

/**
 * Wipe the HttpOnly session cookie for every host it may have been set on.
 * Raw headers are appended because `response.cookies.set` keeps only one
 * entry per cookie name.
 */
export function clearAuthCookies(headers: Headers, hostname: string) {
  for (const name of [AUTH_COOKIE, "oauth_state"]) {
    for (const domain of expireDomains(hostname)) {
      for (const secure of [true, false]) {
        const parts = [
          `${name}=`,
          "Path=/",
          "Max-Age=0",
          "Expires=Thu, 01 Jan 1970 00:00:00 GMT",
          "HttpOnly",
          "SameSite=Lax",
        ];
        if (domain) parts.push(`Domain=${domain}`);
        if (secure) parts.push("Secure");
        headers.append("Set-Cookie", parts.join("; "));
      }
    }
  }
}
