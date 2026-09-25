import { Suspense } from "react";
import { notFound } from "next/navigation";
import { getProject, getProjectSummary } from "@/lib/server-data";
import { ScanClient } from "./scan-client";

export const dynamic = "force-dynamic";

/**
 * The scan screen is server-rendered with the real project so the client stream
 * consumer never has to guess at the domain, then streams the live pipeline.
 */
export default async function ScanPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  const project = await getProject(id);
  if (!project) notFound();

  const summary = await getProjectSummary(id);

  return (
    <Suspense
      fallback={
        <div className="flex min-h-screen items-center justify-center bg-[#f7f3ec] text-sm font-semibold text-zinc-500">
          Starting analysis…
        </div>
      }
    >
      <ScanClient projectId={id} domain={project.domain} initialSummary={summary} />
    </Suspense>
  );
}
