"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { authHeaders, requireUser } from "@/lib/auth";

const API_BASE = process.env.API_BASE_URL || "http://localhost:8080/api";

// ActionState is used by client components (ActionButton, CreateProjectForm)
export type ActionState = { ok: boolean; message: string } | null;

function projectIdOf(payload: unknown): string | number | undefined {
  if (!payload || typeof payload !== "object") return undefined;
  const rec = payload as Record<string, unknown>;
  const nested = rec.project;
  if (nested && typeof nested === "object") {
    const inner = nested as Record<string, unknown>;
    const nestedId = inner.public_id ?? inner.PublicID ?? inner.id ?? inner.ID;
    if (nestedId !== undefined && nestedId !== null) return nestedId as string | number;
  }
  const id = rec.public_id ?? rec.PublicID ?? rec.id ?? rec.ID;
  return id === undefined || id === null ? undefined : (id as string | number);
}

export async function createProjectAction(formData: FormData): Promise<{ error?: string } | undefined> {
  try {
    await requireUser();
  } catch {
    return { error: "Authentication required." };
  }

  const website = String(formData.get("website") ?? "").trim();

  if (!website) {
    return { error: "Website URL is required." };
  }

  const maxPagesRaw = String(formData.get("max_pages") ?? "").trim();
  const maxPages = Number.parseInt(maxPagesRaw, 10);
  const sitemapUrl = String(formData.get("sitemap_url") ?? "").trim();

  let projectId: string | number | undefined;
  let created = false;

  try {
    const res = await fetch(`${API_BASE}/projects`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...(await authHeaders()) },
      body: JSON.stringify({
        website,
        ...(Number.isFinite(maxPages) && maxPages > 0 ? { max_pages: maxPages } : {}),
        ...(sitemapUrl ? { sitemap_url: sitemapUrl } : {}),
      }),
    });

    if (!res.ok) {
      const errText = await res.text();
      try {
        const errJson = JSON.parse(errText);
        return { error: errJson.error || "Failed to create project" };
      } catch {
        return { error: "Failed to create project" };
      }
    }

    created = res.status === 201;
    projectId = projectIdOf(await res.json());
  } catch (e) {
    console.error(`[ACTION] Fetch error:`, e);
    return { error: String(e) };
  }

  revalidatePath("/");
  revalidatePath("/projects");
  if (!created || !projectId) {
    redirect("/projects");
  }
  redirect(`/projects/${projectId}/scan`);
}

/** After Google login: start a scan only when this account has no project yet. */
export async function continueAfterLoginAction(
  website?: string,
): Promise<{ error?: string } | undefined> {
  try {
    await requireUser();
  } catch {
    return { error: "Authentication required." };
  }

  let existingId: string | number | undefined;
  try {
    const listRes = await fetch(`${API_BASE}/projects`, {
      cache: "no-store",
      headers: await authHeaders(),
    });
    if (listRes.ok) {
      const existing = await listRes.json();
      if (Array.isArray(existing) && existing.length > 0) {
        const first = existing[0] as Record<string, unknown>;
        existingId = (first?.id ?? first?.ID ?? first?.PublicID) as string | number | undefined;
      }
    }
  } catch (e) {
    console.error(`[ACTION] List projects error:`, e);
  }

  if (existingId) {
    redirect("/projects");
  }

  const trimmed = (website ?? "").trim();
  if (!trimmed) {
    redirect("/projects");
  }

  const formData = new FormData();
  formData.append("website", trimmed);
  return createProjectAction(formData);
}

export async function setRecommendationStatusAction(
  projectId: string | number,
  recommendationId: number,
  status: "open" | "dismissed" | "done"
) {
  try {
    const res = await fetch(`${API_BASE}/projects/${projectId}/recommendations/${recommendationId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", ...(await authHeaders()) },
      body: JSON.stringify({ status }),
    });
    if (!res.ok) {
      return { ok: false, error: `Failed to update recommendation (${res.status})` };
    }
    revalidatePath(`/projects/${projectId}`);
    return { ok: true, error: undefined };
  } catch (e) {
    return { ok: false, error: String(e) };
  }
}

export async function triggerRecrawlAction(projectId: string | number) {
  try {
    await fetch(`${API_BASE}/projects/${projectId}/recrawl`, {
      method: "POST",
      headers: await authHeaders(),
    });
    revalidatePath(`/projects/${projectId}`);
    return { ok: true };
  } catch (e) {
    return { ok: false, error: String(e) };
  }
}

export async function exportFixAction(projectId: string | number, fixId: number) {
  try {
    await fetch(`${API_BASE}/projects/${projectId}/fixes/${fixId}/export`, {
      method: "POST",
      headers: await authHeaders(),
    });
    revalidatePath(`/projects/${projectId}`);
    return { ok: true };
  } catch (e) {
    return { ok: false, error: String(e) };
  }
}

export async function updateSettingsAction(
  projectId: string | number,
  values: { brand?: string; website?: string; category?: string; country?: string; competitors?: string[] }
) {
  try {
    // The backend stores competitors as a JSON string in a jsonb column.
    const { competitors, ...rest } = values;
    const body = {
      ...rest,
      ...(competitors ? { competitors: JSON.stringify(competitors) } : {}),
    };

    const res = await fetch(`${API_BASE}/projects/${projectId}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...(await authHeaders()) },
      body: JSON.stringify(body),
    });

    if (!res.ok) {
      const text = await res.text();
      return { ok: false, error: text || `Failed to save settings (${res.status})` };
    }

    revalidatePath(`/projects/${projectId}`);
    return { ok: true, error: undefined };
  } catch (e) {
    return { ok: false, error: String(e) };
  }
}

