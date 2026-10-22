import type { ReactNode } from "react";

/** The dashed box a page shows in place of content: nothing yet, nothing matching, or a failed load. */
export function Notice({ children }: { children: ReactNode }) {
  return (
    <div className="space-y-3 rounded-md border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
      {children}
    </div>
  );
}
