"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { ActionState } from "@/app/actions";

/**
 * Client button that calls a server action returning ActionState, shows inline
 * status, and refreshes the page on success.
 */
export function ActionButton({
  action,
  children,
  pendingLabel = "Working…",
  className = "",
  confirmText,
}: {
  action: () => Promise<ActionState>;
  children: React.ReactNode;
  pendingLabel?: string;
  className?: string;
  confirmText?: string;
}) {
  const [pending, startTransition] = useTransition();
  const [state, setState] = useState<ActionState>(null);
  const router = useRouter();

  return (
    <span className="inline-flex flex-col gap-2">
      <button
        type="button"
        disabled={pending}
        className={`b-btn ${className}`}
        onClick={() => {
          if (confirmText && !window.confirm(confirmText)) return;
          setState(null);
          startTransition(async () => {
            const result = await action();
            setState(result);
            if (result?.ok) router.refresh();
          });
        }}
      >
        {pending ? pendingLabel : children}
      </button>
      {state ? (
        <span className={`b-badge ${state.ok ? "b-badge-good" : "b-badge-critical"}`}>
          {state.message}
        </span>
      ) : null}
    </span>
  );
}
