"use client";

import type { ReactNode } from "react";

export function LogoutButton({
  className,
  children = "Log out",
}: {
  className?: string;
  children?: ReactNode;
}) {
  return (
    <a href="/auth/logout" className={className}>
      {children}
    </a>
  );
}
