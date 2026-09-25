import { NextRequest, NextResponse } from "next/server";
import { clearAuthCookies } from "@/lib/clear-auth-cookie";

/** Anyone can open these without a session. Login is a modal on `/`. */
const PUBLIC_PATHS = new Set(["/"]);

/** Logged-out users must not reach these. */
function isPrivatePath(pathname: string) {
  return (
    pathname === "/start" ||
    pathname === "/projects" ||
    pathname.startsWith("/projects/")
  );
}

function isLoggedIn(request: NextRequest) {
  const token = request.cookies.get("visora_token")?.value;
  if (!token) return false;
  try {
    const payload = JSON.parse(
      Buffer.from(token.split(".")[1], "base64url").toString(),
    );
    return !(payload.exp && payload.exp * 1000 < Date.now());
  } catch {
    return false;
  }
}

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  if (
    pathname.startsWith("/_next") ||
    pathname.startsWith("/favicon") ||
    pathname.startsWith("/auth") ||
    pathname.startsWith("/api/") ||
    pathname.startsWith("/go-api")
  ) {
    return NextResponse.next();
  }

  const loggingOut = request.nextUrl.searchParams.get("logged_out") === "1";
  const loggedIn = !loggingOut && isLoggedIn(request);
  const to = (path: string) => NextResponse.redirect(new URL(path, request.url));

  // Logout landing — wipe leftovers and never bounce back into /projects.
  if (loggingOut) {
    const res = pathname === "/" ? NextResponse.next() : to("/?logged_out=1");
    clearAuthCookies(res, request.nextUrl.hostname);
    res.headers.set("Cache-Control", "no-store, no-cache, must-revalidate");
    return res;
  }

  // Removed routes — send people to the right side of the gate.
  if (pathname === "/onboarding" || pathname === "/login") {
    return to(loggedIn ? "/projects" : "/");
  }

  if (PUBLIC_PATHS.has(pathname)) {
    if (loggedIn) return to("/projects");
    return NextResponse.next();
  }

  if (isPrivatePath(pathname) && !loggedIn) {
    return to("/");
  }

  // Anything else is private app UI.
  if (!loggedIn) {
    return to("/");
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
