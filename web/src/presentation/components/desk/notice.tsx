import type { ReactNode } from "react";

export function Notice({ children }: { children: ReactNode }) {
  return (
    <div className="space-y-3 rounded-md border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
      {children}
    </div>
  );
}
