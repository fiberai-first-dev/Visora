"use client";

import { useEffect, useState } from "react";

/** Post-login router: existing project → /projects, first site → scan. */
export default function StartPage() {
  const [error, setError] = useState("");

  useEffect(() => {
    const pending = sessionStorage.getItem("pending_website") ?? "";
    sessionStorage.removeItem("pending_website");

    void (async () => {
      try {
        const { continueAfterLoginAction } = await import("@/app/actions");
        const result = await continueAfterLoginAction(pending);
        if (result?.error) setError(result.error);
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : "";
        if (message.includes("NEXT_REDIRECT")) return;
        setError(message || "Could not open your workspace.");
      }
    })();
  }, []);

  return (
    <div className="analysis-surface flex min-h-screen flex-col items-center justify-center px-6">
      <p className="font-[family-name:var(--font-display)] text-3xl text-[var(--ink)]">
        {error || "Opening your workspace…"}
      </p>
    </div>
  );
}
