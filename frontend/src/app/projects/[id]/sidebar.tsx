"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { LogoutButton } from "@/components/LogoutButton";

const NAV = [
  { name: "Overview", path: "", hint: "Scores & next steps" },
  { name: "What to fix", path: "/fixes", hint: "Paste-ready changes" },
  { name: "Google", path: "/visibility", hint: "Where you rank" },
  { name: "AI answers", path: "/geo", hint: "Where AIs mention you" },
  { name: "Competitors", path: "/competitors", hint: "Who shows up" },
  { name: "Pages", path: "/pages", hint: "Crawled URLs" },
  { name: "Site", path: "/seo", hint: "Technical health" },
  { name: "Settings", path: "/settings", hint: "Project setup" },
] as const;

export default function Sidebar({
  projectId,
  brandName,
  domain,
}: {
  projectId: string;
  brandName: string;
  domain: string;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const base = `/projects/${projectId}`;

  return (
    <aside className="sticky top-0 z-10 flex h-screen w-[15.5rem] shrink-0 flex-col border-r border-zinc-200/80 bg-[#f7f4ef]">
      <div className="border-b border-zinc-200/80 px-4 py-5">
        <h2 className="truncate font-[family-name:var(--font-display)] text-xl leading-tight text-[#1a1a18]">
          Visora
        </h2>
        <p
          className="mt-0.5 truncate font-mono text-[11px] text-zinc-500"
          title={brandName ? `${brandName} · ${domain}` : domain}
        >
          {domain || brandName || "Project"}
        </p>
      </div>

      <nav className="flex flex-1 flex-col gap-1 overflow-y-auto px-2.5 py-4">
        {NAV.map((item) => {
          const href = `${base}${item.path}`;
          const isActive =
            item.path === ""
              ? pathname === base || pathname === `${base}/`
              : pathname === href || pathname.startsWith(`${href}/`);
          return (
            <Link
              key={item.name}
              href={href}
              aria-current={isActive ? "page" : undefined}
              className={`group rounded-xl px-3 py-2.5 transition ${
                isActive
                  ? "bg-[#1a1a18] text-white shadow-sm"
                  : "text-zinc-600 hover:bg-white/80 hover:text-[#1a1a18]"
              }`}
            >
              <span className="block text-[13px] font-semibold tracking-tight">{item.name}</span>
              <span
                className={`mt-0.5 block text-[11px] ${
                  isActive ? "text-white/50" : "text-zinc-400 group-hover:text-zinc-500"
                }`}
              >
                {item.hint}
              </span>
            </Link>
          );
        })}
      </nav>

      <div className="border-t border-zinc-200/80 p-3">
        <button
          type="button"
          onClick={() => router.push(`${base}/scan?rerun=1&t=${Date.now()}`)}
          className="flex w-full items-center justify-center rounded-xl bg-[#1a1a18] px-3 py-3 text-[11px] font-semibold uppercase tracking-[0.12em] text-white transition hover:bg-[#a65a3a]"
        >
          Re-run analysis
        </button>
      </div>

      <div className="border-t border-zinc-200/80 p-3">
        <LogoutButton className="block w-full text-left rounded-xl px-3 py-2.5 transition text-rose-600 font-medium text-[13px] hover:bg-rose-50" />
      </div>
    </aside>
  );
}
