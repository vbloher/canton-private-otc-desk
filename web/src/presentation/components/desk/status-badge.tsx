import { Badge } from "@/presentation/components/ui/badge";
import { cn } from "@/presentation/lib/utils";

const TONE: Record<string, string> = {
  open: "border-border text-muted-foreground",
  new: "border-primary/60 text-primary",
  allocating: "border-primary/50 text-primary",
  accepted: "border-primary/50 text-primary",
  settling: "border-primary/50 text-primary",
  won: "border-success text-success",
  filled: "border-success text-success",
  settled: "border-success text-success",
  failed: "border-destructive/60 text-destructive",
};

const LABEL: Record<string, string> = {
  new: "quote now",
  allocating: "locking funds",
  accepted: "settling",
};

/** A status word in the desk's accent: amber in flight, green done, red failed, muted otherwise. */
export function StatusBadge({ status, className }: { status: string; className?: string }) {
  return (
    <Badge
      variant="outline"
      className={cn("tracking-wide uppercase", TONE[status] ?? "border-border text-muted-foreground", className)}
    >
      {LABEL[status] ?? status}
    </Badge>
  );
}
