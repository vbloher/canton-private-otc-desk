/**
 * A trade from accept to atomic settlement: what already happened, what this
 * browser is signing, what the desk is waiting for — and, once settled, what
 * moved, with the Canton update id as proof.
 */
import { Check, Loader2, X } from "lucide-react";
import type { ReactNode } from "react";
import { viewTrade } from "@/domain/trade";
import type { Quote, Rfq, Trade } from "@/domain/types";
import { Button } from "@/presentation/components/ui/button";
import { useInstrument, useUnlock } from "@/presentation/hooks/queries";
import { formatAmount, formatDateTime, shortId } from "@/presentation/lib/format";
import { cn } from "@/presentation/lib/utils";
import { useSession } from "@/presentation/providers/session";
import { useSigner } from "@/presentation/providers/signer";
import { CopyButton } from "./copy-button";
import { DetailRow } from "./detail-row";

type StepState = "done" | "active" | "failed";

export function TradeStatus({
  rfq,
  quote,
  trade,
  role,
}: {
  rfq: Rfq;
  /** The accepted (or won) quote. */
  quote?: Quote;
  trade?: Trade;
  role: "taker" | "maker";
}) {
  const { me, locked } = useSession();
  const { pending } = useSigner();
  const { unlock, unlocking } = useUnlock();
  const instrument = useInstrument();
  const settled = trade?.status === "settled";
  const failed = trade?.status === "failed";
  // The taker's allocation is the only signature a settlement needs from this browser.
  const signing = role === "taker" && !settled && !failed && pending.some((a) => a.purpose === "allocate");
  const price = trade?.price ?? quote?.price;
  const maker = trade?.maker ?? quote?.maker;

  return (
    <div className="space-y-4">
      <ol className="pl-6 text-sm">
        <Step
          state="done"
          label={
            role === "taker"
              ? `Quote accepted${maker === undefined ? "" : ` from ${maker}`}${price === undefined ? "" : ` at ${formatAmount(price)} ${rfq.quote}`}`
              : `The taker accepted your quote${price === undefined ? "" : ` at ${formatAmount(price)} ${rfq.quote}`}`
          }
          sub="the price is locked"
        />
        {role === "taker" ? (
          <Step
            state={signing ? "active" : "done"}
            label={
              signing
                ? locked
                  ? "Waiting for your signature to allocate your funds"
                  : "Signing your allocation…"
                : "Your funds are allocated for the settlement"
            }
            sub={
              signing && locked ? (
                <Button size="xs" className="mt-1" disabled={unlocking} onClick={() => unlock()}>
                  Unlock to sign
                </Button>
              ) : (
                "the maker's funds were committed with the quote"
              )
            }
          />
        ) : (
          <Step state="done" label="Your funds were committed when you quoted" sub="no signature needed now" />
        )}
        {failed ? (
          <Step state="failed" label="Settlement failed — nothing moved" sub={trade.reason ?? "no reason given"} />
        ) : settled ? (
          <Step
            state="done"
            label="Settled atomically"
            sub={trade.settledAt === undefined ? undefined : formatDateTime(trade.settledAt)}
          />
        ) : (
          <Step state="active" label="Settling both legs in one Canton transaction…" sub="no action needed" />
        )}
      </ol>

      {settled && me !== null && <Summary trade={trade} myHint={me.hint} decimalsOf={(s) => instrument(s).decimals} />}
    </div>
  );
}

function Summary({ trade, myHint, decimalsOf }: { trade: Trade; myHint: string; decimalsOf: (symbol: string) => number }) {
  const view = viewTrade(trade, myHint);
  const amount = (a: { symbol: string; amount: string }): string =>
    `${formatAmount(a.amount, decimalsOf(a.symbol))} ${a.symbol}`;
  return (
    <div className="space-y-1.5 rounded-md border border-border bg-secondary/50 p-4 text-sm">
      <DetailRow label="You received" value={`+${amount(view.got)}`} />
      <DetailRow label="You sent" value={`−${amount(view.gave)}`} />
      {view.fee !== undefined && <DetailRow label="Desk fee" value={`−${amount(view.fee)}`} />}
      <DetailRow label="Price" value={`${formatAmount(trade.price)} ${trade.quote} per ${trade.base}`} />
      <DetailRow label={view.role === "taker" ? "Maker" : "Taker"} value={view.counterparty} />
      {trade.updateId !== undefined && (
        <div className="flex items-center justify-between gap-4 border-t border-border pt-2">
          <span className="text-muted-foreground">Canton update id</span>
          <span className="flex items-center gap-1.5">
            <span className="tabular font-medium" title={trade.updateId}>
              {shortId(trade.updateId)}
            </span>
            <CopyButton value={trade.updateId} label="Copy update id" />
          </span>
        </div>
      )}
      <p className="pt-1 text-xs text-muted-foreground">
        One atomic transaction: both legs moved together. Makers who lost only saw &ldquo;lost&rdquo; — never the price.
      </p>
    </div>
  );
}

function Step({ state, label, sub }: { state: StepState; label: string; sub?: ReactNode }) {
  return (
    <li className="group relative pb-4 last:pb-0">
      <span aria-hidden className="absolute top-5 bottom-0 -left-[15px] w-px bg-border group-last:hidden" />
      <span
        aria-hidden
        className={cn(
          "absolute top-0.5 -left-6 flex size-[18px] items-center justify-center rounded-full border",
          state === "done" && "border-success/60 text-success",
          state === "active" && "border-primary/60 text-primary",
          state === "failed" && "border-destructive/60 text-destructive",
        )}
      >
        {state === "done" && <Check className="size-3" />}
        {state === "active" && <Loader2 className="size-3 animate-spin" />}
        {state === "failed" && <X className="size-3" />}
      </span>
      <p className={cn(state === "failed" && "text-destructive")}>{label}</p>
      {sub !== undefined && <div className="text-xs text-muted-foreground">{sub}</div>}
    </li>
  );
}
