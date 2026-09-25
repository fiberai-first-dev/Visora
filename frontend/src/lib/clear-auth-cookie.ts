import { NextResponse } from "next/server";

export const AUTH_COOKIE = "visora_token";

function expireDomains(hostname: string): (string | undefined)[] {
  const hosts = new Set<string | undefined>([undefined]);
  const add = (raw?: string) => {
    const d = (raw || "").replace(/^\./, "").trim();
    if (!d) return;
    hosts.add(d);
    hosts.add(`.${d}`);
  };
  add(hostname);
  const parts = hostname.split(".").filter(Boolean);
  for (let i = 0; i < parts.length - 1; i++) {
    add(parts.slice(i).join("."));
  }
  return [...hosts];
}

/** Wipe the HttpOnly session cookie for every host it may have been set on. */
export function clearAuthCookies(response: NextResponse, hostname: string) {
  for (const domain of expireDomains(hostname)) {
    for (const secure of [true, false]) {
      response.cookies.set({
        name: AUTH_COOKIE,
        value: "",
        path: "/",
        maxAge: 0,
        expires: new Date(0),
        httpOnly: true,
        sameSite: "lax",
        secure,
        ...(domain ? { domain } : {}),
      });
    }
  }
}
