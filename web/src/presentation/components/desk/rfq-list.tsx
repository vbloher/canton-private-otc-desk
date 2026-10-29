import { bestQuote, incomingRowStatus, rfqRowStatus } from "@/domain/rfq";
import type { IncomingRfq, Rfq, RfqWithQuotes } from "@/domain/types";
import { useInstrument } from "@/presentation/hooks/queries";
import { useNow } from "@/presentation/hooks/use-now";
import { count, formatAmount } from "@/presentation/lib/format";
import { cn } from "@/presentation/lib/utils";
import { StatusBadge } from "./status-badge";

const rowClass = (active: boolean): string =>
  cn(
    "w-full rounded-md border p-3 text-left transition-colors",
    active ? "border-primary/60 bg-card" : "border-border hover:bg-card",
  );

export function RfqTitle({ rfq, className }: { rfq: Rfq; className?: string }) {
  const instrument = useInstrument();
  const buy = rfq.direction === "BUY";
  return (
    <span className={cn("font-medium", className)}>
      <span className={buy ? "text-success" : "text-destructive"}>{rfq.direction}</span>{" "}
      <span className="tabular">{formatAmount(rfq.qty, instrument(rfq.base).decimals)}</span> {rfq.base}{" "}
      <span className="text-muted-foreground">
        {buy ? "with" : "for"} {rfq.quote}
      </span>
    </span>
  );
}

export function MyRfqList({
  entries,
  selectedId,
  onSelect,
}: {
  entries: RfqWithQuotes[];
  selectedId: string | undefined;
  onSelect: (id: string) => void;
}) {
  const now = useNow();
  return (
    <div className="space-y-2">
      {entries.map((entry) => {
        const status = rfqRowStatus(entry, now);
        const live = entry.quotes.filter((q) => q.status === "open" && q.validUntil > now);
        const best = bestQuote(entry.quotes, entry.rfq.direction, now);
        return (
          <button
            key={entry.rfq.id}
            type="button"
            aria-current={entry.rfq.id === selectedId}
            className={rowClass(entry.rfq.id === selectedId)}
            onClick={() => onSelect(entry.rfq.id)}
          >
            <div className="flex items-center justify-between gap-2">
              <RfqTitle rfq={entry.rfq} className="text-sm" />
              <StatusBadge status={status} />
            </div>
            <p className="mt-1 text-xs text-muted-foreground">
              {status === "open"
                ? `${count(live.length, "live quote", "live quotes")}${best === undefined ? "" : ` · best ${formatAmount(best.price)}`}`
                : count(entry.quotes.length, "quote", "quotes")}
              {" · "}
              {count(entry.rfq.makers.length, "maker", "makers")} asked
            </p>
          </button>
        );
      })}
    </div>
  );
}

export function IncomingRfqList({
  entries,
  selectedId,
  onSelect,
}: {
  entries: IncomingRfq[];
  selectedId: string | undefined;
  onSelect: (id: string) => void;
}) {
  const now = useNow();
  return (
    <div className="space-y-2">
      {entries.map((entry) => (
        <button
          key={entry.rfq.id}
          type="button"
          aria-current={entry.rfq.id === selectedId}
          className={rowClass(entry.rfq.id === selectedId)}
          onClick={() => onSelect(entry.rfq.id)}
        >
          <div className="flex items-center justify-between gap-2">
            <RfqTitle rfq={entry.rfq} className="text-sm" />
            <StatusBadge status={incomingRowStatus(entry, now)} />
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            from {entry.rfq.taker} · fee {entry.rfq.feeBps} bps if you win
          </p>
        </button>
      ))}
    </div>
  );
}
