import { clearAuthCookies, publicHost } from "@/lib/clear-auth-cookie";

export const dynamic = "force-dynamic";

/** Stay on this host. Expire the HttpOnly session cookie, then land on `/`. */
export async function GET(request: Request) {
  const headers = new Headers({
    // Relative, so the browser stays on the public host rather than the container's.
    Location: "/?logged_out=1",
    "Cache-Control": "no-store, no-cache, must-revalidate",
  });
  clearAuthCookies(headers, publicHost(request.headers));
  return new Response(null, { status: 303, headers });
}
