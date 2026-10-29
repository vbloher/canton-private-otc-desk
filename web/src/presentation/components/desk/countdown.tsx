import { Progress } from "@/presentation/components/ui/progress";
import { useNow } from "@/presentation/hooks/use-now";
import { formatCountdown } from "@/presentation/lib/format";

export function Countdown({ from, until }: { from: number; until: number }) {
  const now = useNow();
  const left = Math.max(0, until - now);
  const total = Math.max(1, until - from);
  return (
    <div className="flex items-center gap-2" title={new Date(until).toLocaleString()}>
      <Progress value={Math.min(100, (left / total) * 100)} className="h-1.5 w-24" />
      <span className="w-14 text-xs text-muted-foreground tabular-nums">{left === 0 ? "ended" : formatCountdown(left)}</span>
    </div>
  );
}

export function TimeLeft({ until, prefix = "" }: { until: number; prefix?: string }) {
  const left = Math.max(0, until - useNow());
  return (
    <span className="text-xs text-muted-foreground tabular-nums">
      {left === 0 ? "expired" : `${prefix}${formatCountdown(left)}`}
    </span>
  );
}
