import { NextResponse } from "next/server";
import { clearAuthCookies } from "@/lib/clear-auth-cookie";

/** Stay on this host. Expire the HttpOnly session cookie, then land on `/`. */
export async function GET(request: Request) {
  const url = new URL(request.url);
  const dest = new URL("/?logged_out=1", url.origin);
  const response = NextResponse.redirect(dest, 303);
  clearAuthCookies(response, url.hostname);
  response.headers.set("Cache-Control", "no-store, no-cache, must-revalidate");
  return response;
}
