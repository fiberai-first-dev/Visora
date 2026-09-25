import { NextRequest, NextResponse } from "next/server";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const UPSTREAM = (process.env.API_BASE_URL || "http://localhost:8080/api").replace(/\/$/, "");

async function proxy(req: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const { path } = await context.params;
  const target = `${UPSTREAM}/${path.join("/")}${req.nextUrl.search}`;

  const headers = new Headers();
  const cookie = req.headers.get("cookie");
  const contentType = req.headers.get("content-type");
  if (cookie) headers.set("cookie", cookie);
  if (contentType) headers.set("content-type", contentType);
  const email = req.headers.get("x-user-email");
  if (email) headers.set("x-user-email", email);

  const init: RequestInit = {
    method: req.method,
    headers,
    cache: "no-store",
    redirect: "manual",
  };
  if (req.method !== "GET" && req.method !== "HEAD") {
    init.body = await req.arrayBuffer();
  }

  const upstream = await fetch(target, init);
  const out = new Headers();
  const pass = [
    "content-type",
    "cache-control",
    "connection",
    "x-accel-buffering",
    "content-security-policy",
    "x-frame-options",
  ];
  for (const key of pass) {
    const value = upstream.headers.get(key);
    if (value) out.set(key, value);
  }

  return new NextResponse(upstream.body, {
    status: upstream.status,
    headers: out,
  });
}

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
