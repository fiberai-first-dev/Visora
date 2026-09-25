"use client";

import { useAuthModal } from "@/components/AuthModal";

export function LoginButton({ className }: { className?: string }) {
  const { openAuthModal } = useAuthModal();
  return (
    <button
      type="button"
      onClick={() =>
        openAuthModal(typeof window !== "undefined" ? `${window.location.origin}/start` : "/start")
      }
      className={className}
    >
      Log in
    </button>
  );
}
