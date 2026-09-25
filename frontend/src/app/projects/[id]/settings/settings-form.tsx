import { ApiProject } from "@/lib/data";

/** Read-only project profile — brand, site, category, and market. */
export function SettingsForm({ project }: { project: ApiProject }) {
  const rows = [
    { label: "Brand name", value: project.brand || "—" },
    { label: "Website URL", value: project.website || "—" },
    { label: "Category", value: project.category || "—" },
    { label: "Market", value: "India" },
  ];

  return (
    <div className="b-card flex flex-col gap-5 p-5">
      {rows.map((row) => (
        <div key={row.label}>
          <span className="b-label mb-2 block">{row.label}</span>
          <div className="b-input flex items-center bg-zinc-100 text-zinc-700" aria-readonly>
            {row.value}
          </div>
        </div>
      ))}
    </div>
  );
}
