import { useMemo, useState } from "react";
import { activityFeed, type ActivityItem } from "@/domain/trade";
import { CopyButton } from "@/presentation/components/desk/copy-button";
import { Notice } from "@/presentation/components/desk/notice";
import { RfqTitle } from "@/presentation/components/desk/rfq-list";
import { StatusBadge } from "@/presentation/components/desk/status-badge";
import { TokenIcon } from "@/presentation/components/desk/token-icon";
import { Button } from "@/presentation/components/ui/button";
import { RadioGroup, RadioGroupItem } from "@/presentation/components/ui/radio-group";
import { Skeleton } from "@/presentation/components/ui/skeleton";
import { useInstrument, useRfqHistory, useTrades } from "@/presentation/hooks/queries";
import { useNow } from "@/presentation/hooks/use-now";
import { groupByDay } from "@/presentation/lib/days";
import { count, formatAmount, formatDateTime, formatTime, shortId } from "@/presentation/lib/format";
import { cn } from "@/presentation/lib/utils";
import { useSession } from "@/presentation/providers/session";

type Filter = "all" | "trades" | "unfilled";

const FILTERS: { value: Filter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "trades", label: "Trades" },
  { value: "unfilled", label: "Not filled" },
];

export function ActivityPage() {
  const { me } = useSession();
  const trades = useTrades();
  const history = useRfqHistory();
  const now = useNow();
  const [filter, setFilter] = useState<Filter>("all");

  const feed = useMemo(
    () => activityFeed(trades.data ?? [], history.data ?? [], me?.hint ?? ""),
    [trades.data, history.data, me?.hint],
  );
  const shown = feed.filter((i) => filter === "all" || (filter === "trades" ? i.kind === "trade" : i.kind === "rfq"));
  const groups = groupByDay(shown, now);
  const tradeCount = feed.filter((i) => i.kind === "trade").length;

  if (trades.isPending || history.isPending) {
    return (
      <div className="mx-auto w-full max-w-4xl space-y-2" aria-busy>
        <Skeleton className="h-18 rounded-lg" />
        <Skeleton className="h-18 rounded-lg" />
        <Skeleton className="h-18 rounded-lg" />
      </div>
    );
  }
  if ((trades.isError && trades.data === undefined) || (history.isError && history.data === undefined)) {
    return (
      <div className="mx-auto w-full max-w-4xl">
        <Notice>
          <p>Could not load your activity.</p>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void trades.refetch();
              void history.refetch();
            }}
          >
            Try again
          </Button>
        </Notice>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-4xl space-y-6">
      {feed.length === 0 ? (
        <Notice>No activity yet — your trades and the requests that did not fill land here.</Notice>
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <RadioGroup
              value={filter}
              onValueChange={(v) => setFilter(FILTERS.find((f) => f.value === v)?.value ?? "all")}
              aria-label="Show"
              className="inline-flex rounded-lg border border-border bg-card/50 p-0.5"
            >
              {FILTERS.map((f) => (
                <RadioGroupItem
                  key={f.value}
                  value={f.value}
                  className="rounded-md px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground data-checked:bg-secondary data-checked:text-foreground"
                >
                  {f.label}
                </RadioGroupItem>
              ))}
            </RadioGroup>
            <span className="text-xs text-muted-foreground">
              {count(tradeCount, "trade", "trades")}, {count(feed.length - tradeCount, "request", "requests")} not filled
            </span>
          </div>
          {shown.length === 0 ? (
            <Notice>Nothing here for this filter.</Notice>
          ) : (
            <div className="space-y-6">
              {groups.map((g) => (
                <section key={g.key} aria-label={g.label} className="space-y-2">
                  <h2 className="text-sm font-medium text-muted-foreground">{g.label}</h2>
                  {g.items.map((item) => (
                    <ActivityCard key={item.key} item={item} />
                  ))}
                </section>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function ActivityCard({ item }: { item: ActivityItem }) {
  return item.kind === "trade" ? <TradeCard item={item} /> : <UnfilledCard item={item} />;
}

function PairIcons({ from, to }: { from: string; to: string }) {
  return (
    <span aria-hidden className="relative h-8 w-[3.25rem] flex-none">
      <TokenIcon symbol={from} className="absolute top-0 left-0" />
      <TokenIcon symbol={to} className="absolute top-0 left-5 ring-2 ring-card" />
    </span>
  );
}

function TradeCard({ item }: { item: Extract<ActivityItem, { kind: "trade" }> }) {
  const instrument = useInstrument();
  const { trade: t, view } = item;
  const amount = (leg: { symbol: string; amount: string }): string =>
    `${formatAmount(leg.amount, instrument(leg.symbol).decimals)} ${leg.symbol}`;
  return (
    <article className="flex flex-col gap-3 rounded-lg border border-border bg-card px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
      <div className="flex min-w-0 items-center gap-3">
        <PairIcons from={view.gave.symbol} to={view.got.symbol} />
        <div className="min-w-0">
          <p className="font-medium">
            <span className={view.boughtBase ? "text-success" : "text-destructive"}>
              {view.boughtBase ? "Bought" : "Sold"}
            </span>{" "}
            {t.base} {view.boughtBase ? "with" : "for"} {t.quote}
          </p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            <span className="tabular">
              {formatAmount(t.price)} {t.quote} per {t.base}
            </span>{" "}
            · {view.role === "taker" ? "maker" : "taker"} {view.counterparty}
          </p>
          {t.status === "failed" && (
            <p className="mt-0.5 text-xs text-destructive">Settlement failed: {t.reason ?? "no reason given"}</p>
          )}
        </div>
      </div>
      <div className="flex items-end justify-between gap-4 sm:flex-col sm:items-end sm:gap-1">
        <div className={cn("tabular text-sm sm:text-right", t.status === "failed" && "line-through opacity-60")}>
          <p className="font-medium">+{amount(view.got)}</p>
          <p className="text-muted-foreground">−{amount(view.gave)}</p>
          {view.fee !== undefined && <p className="text-xs text-muted-foreground">−{amount(view.fee)} fee</p>}
        </div>
        {t.status === "settled" ? (
          <span className="flex items-center gap-1.5 text-xs whitespace-nowrap text-muted-foreground">
            <time dateTime={new Date(item.at).toISOString()} title={formatDateTime(item.at)} className="tabular">
              {formatTime(item.at)}
            </time>
            {t.updateId !== undefined && (
              <>
                <span aria-hidden>·</span>
                <span className="tabular" title={`Canton update id ${t.updateId}`}>
                  {shortId(t.updateId)}
                </span>
                <CopyButton value={t.updateId} label="Copy update id" size="icon-xs" />
              </>
            )}
          </span>
        ) : (
          <StatusBadge status={t.status} />
        )}
      </div>
    </article>
  );
}

function UnfilledCard({ item }: { item: Extract<ActivityItem, { kind: "rfq" }> }) {
  const { rfq } = item;
  const buy = rfq.direction === "BUY";
  return (
    <article className="flex flex-col gap-3 rounded-lg border border-dashed border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
      <div className="flex min-w-0 items-center gap-3">
        <PairIcons from={buy ? rfq.quote : rfq.base} to={buy ? rfq.base : rfq.quote} />
        <div className="min-w-0">
          <RfqTitle rfq={rfq} />
          <p className="mt-0.5 text-xs text-muted-foreground">
            {count(rfq.makers.length, "maker", "makers")} asked: {rfq.makers.join(", ")}
          </p>
        </div>
      </div>
      <div className="flex items-center justify-between gap-3 sm:flex-col sm:items-end sm:gap-1">
        <StatusBadge status={rfq.status} />
        <time dateTime={new Date(item.at).toISOString()} title={formatDateTime(item.at)} className="tabular text-xs text-muted-foreground">
          {formatTime(item.at)}
        </time>
      </div>
    </article>
  );
}
