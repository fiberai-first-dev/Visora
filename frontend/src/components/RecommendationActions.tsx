"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { setRecommendationStatusAction } from "@/app/actions";

export function RecommendationActions({
  projectId,
  recommendationId,
}: {
  projectId: string | number;
  recommendationId: number;
}) {
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const router = useRouter();

  const update = (status: "dismissed" | "done") => {
    setError(null);
    startTransition(async () => {
      const result = await setRecommendationStatusAction(projectId, recommendationId, status);
      if (result && !result.ok) {
        setError(result.error ?? "Could not update this recommendation.");
        return;
      }
      router.refresh();
    });
  };

  return (
    <div className="flex shrink-0 flex-col items-end gap-2">
      <div className="flex gap-2">
        <button
          type="button"
          disabled={pending}
          onClick={() => update("done")}
          className="b-btn b-btn-sm"
          style={{ background: "var(--brutal-emerald)" }}
        >
          ✓ Done
        </button>
        <button
          type="button"
          disabled={pending}
          onClick={() => update("dismissed")}
          className="b-btn b-btn-sm"
        >
          Dismiss
        </button>
      </div>
      {error ? <span className="b-badge b-badge-critical">{error}</span> : null}
    </div>
  );
}
