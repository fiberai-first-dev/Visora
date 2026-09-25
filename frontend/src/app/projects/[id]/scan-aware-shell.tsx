"use client";

import type { ReactNode } from "react";
import { usePathname } from "next/navigation";
import Sidebar from "./sidebar";

/** Hides the product sidebar while the analysis journey is on screen. */
export function ScanAwareShell({
  children,
  projectId,
  brandName,
  domain,
}: {
  children: ReactNode;
  projectId: string;
  brandName: string;
  domain: string;
}) {
  const pathname = usePathname();
  const isScan = pathname?.endsWith("/scan") ?? false;

  if (isScan) {
    return <div className="min-h-screen">{children}</div>;
  }

  return (
    <div className="flex min-h-screen bg-[#f3eee6]">
      <Sidebar projectId={projectId} brandName={brandName} domain={domain} />
      <main className="min-w-0 flex-1">
        <div className="min-h-full bg-[#faf8f4]">{children}</div>
      </main>
    </div>
  );
}
