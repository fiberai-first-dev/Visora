import Link from "next/link";
import { Badge, Card, EmptyState, SectionHeader } from "@/components/ui";
import { DomainIntake } from "@/components/DomainIntake";
import { LogoutButton } from "@/components/LogoutButton";
import { formatDate } from "@/lib/data";
import { listProjects } from "@/lib/server-data";

export const dynamic = "force-dynamic";

/**
 * One project per account. Empty accounts can enter a site here;
 * everyone else opens the project they already have.
 */
export default async function ProjectsPage() {
  const projects = await listProjects();

  return (
    <div className="b-surface min-h-screen p-6 md:p-10">
      <div className="mx-auto max-w-6xl">
        <header className="mb-8 flex flex-wrap items-end justify-between gap-4">
          <div>
            <span className="b-kicker text-zinc-500">Workspace</span>
            <h1 className="b-display text-4xl">Your project</h1>
            <p className="mt-1 text-sm font-bold text-zinc-600">
              {projects.length === 0
                ? "Add your website to start a scan."
                : "Your site and what we’re tracking."}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <LogoutButton className="b-btn b-btn-sm !text-rose-600" />
          </div>
        </header>

        {projects.length === 0 ? (
          <EmptyState
            title="No project yet"
            hint="Enter your website — we’ll start the scan right away."
            action={
              <div className="w-full max-w-2xl">
                <DomainIntake compact cta="Start scan" />
              </div>
            }
          />
        ) : (
          <section>
            <SectionHeader
              title="Your site"
              description="Open the dashboard or watch the live scan."
            />
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
              {projects.map((project) => (
                <Card key={project.id} className="flex flex-col gap-3">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="min-w-0">
                      <h2 className="b-display truncate text-lg" title={project.brand}>
                        {project.brand}
                      </h2>
                      <p className="b-mono truncate text-xs font-bold text-zinc-500" title={project.domain}>
                        {project.domain || project.website || "no website"}
                      </p>
                    </div>
                    <Badge tone={project.status === "active" ? "good" : "neutral"}>
                      {project.status}
                    </Badge>
                  </div>

                  <div className="flex flex-wrap gap-2">
                    <span className="b-badge b-badge-neutral">{project.category}</span>
                    <span className="b-badge b-badge-neutral">{project.country}</span>
                  </div>

                  <p className="text-xs font-bold text-zinc-500">
                    Created {formatDate(project.createdAt)}
                  </p>

                  <div className="mt-auto flex flex-wrap gap-2">
                    <Link href={`/projects/${project.id}`} className="b-btn b-btn-sm b-btn-primary">
                      Open dashboard
                    </Link>
                    <Link href={`/projects/${project.id}/scan`} className="b-btn b-btn-sm">
                      Live scan
                    </Link>
                  </div>
                </Card>
              ))}
            </div>
          </section>
        )}
      </div>
    </div>
  );
}
