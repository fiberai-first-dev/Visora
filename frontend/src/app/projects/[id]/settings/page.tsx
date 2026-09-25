import { notFound } from "next/navigation";
import { Card, SectionHeader } from "@/components/ui";
import { formatDate } from "@/lib/data";
import { getProject, getProjectSummary } from "@/lib/server-data";
import { SettingsForm } from "./settings-form";

export const dynamic = "force-dynamic";

/** Project settings — profile values set at create / scan time. */
export default async function SettingsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  const project = await getProject(id);
  if (!project) notFound();

  const summary = await getProjectSummary(id);

  const facts = [
    { label: "Domain", value: project.domain || "—" },
    { label: "Status", value: project.status },
    { label: "Created", value: formatDate(project.createdAt) },
    { label: "Pages stored", value: String(summary?.counts.pages ?? 0) },
    { label: "Tracked keywords", value: String(summary?.counts.keywords ?? 0) },
  ];

  return (
    <div className="b-surface flex flex-col gap-8 p-6 md:p-8">
      <header>
        <span className="b-kicker text-zinc-500">Settings</span>
        <h1 className="b-display text-3xl">{project.brand || project.domain}</h1>
        <p className="mt-1 text-sm font-bold text-zinc-600">Project profile</p>
      </header>

      <section className="grid gap-6 lg:grid-cols-2">
        <div>
          <SectionHeader title="Profile" />
          <SettingsForm project={project} />
        </div>

        <div>
          <SectionHeader title="Activity" />
          <Card className="flex flex-col">
            {facts.map((fact) => (
              <div
                key={fact.label}
                className="flex items-start justify-between gap-4 py-2 text-sm"
                style={{ borderBottomWidth: 2, borderBottomStyle: "solid", borderColor: "#e4e4e7" }}
              >
                <span className="b-label shrink-0 text-zinc-500">{fact.label}</span>
                <span className="b-mono break-all text-right text-xs font-bold">{fact.value}</span>
              </div>
            ))}
          </Card>
        </div>
      </section>
    </div>
  );
}
