import { cn } from "@/presentation/lib/utils";

/** The desk's mark: an amber tile with two arrows trading places. */
export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" role="img" aria-label="Private OTC Desk" className={cn("size-7", className)}>
      <rect width="32" height="32" rx="8" fill="var(--primary)" />
      <g
        fill="none"
        stroke="var(--primary-foreground)"
        strokeWidth="2.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M8 12h15l-4-4" />
        <path d="M24 20H9l4 4" />
      </g>
    </svg>
  );
}
