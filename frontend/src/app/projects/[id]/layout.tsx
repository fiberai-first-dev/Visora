export const dynamic = "force-dynamic";

import type { ReactNode } from "react";
import { notFound } from "next/navigation";
import { getProject } from "@/lib/server-data";
import { ScanAwareShell } from "./scan-aware-shell";

export default async function ProjectLayout({
  children,
  params: paramsPromise,
}: {
  children: ReactNode;
  params: Promise<{ id: string }>;
}) {
  const { id } = await paramsPromise;

  const project = await getProject(id);
  if (!project) notFound();

  return (
    <ScanAwareShell projectId={id} brandName={project.brand} domain={project.domain}>
      {children}
    </ScanAwareShell>
  );
}
