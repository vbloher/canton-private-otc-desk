/**
 * The detail pane of the Trade page. A taker's RFQ walks quotes → accept →
 * atomic settlement; an incoming RFQ is the maker's quote form, then their
 * quote's fate.
 */
import type { ReactNode } from "react";
import { toast } from "sonner";
import { bestQuote, canAccept, canCancel, canQuote, incomingRowStatus, makerTerms, quoteTerms, rfqRowStatus } from "@/domain/rfq";
import type { IncomingRfq, Quote, Rfq, RfqWithQuotes, Trade } from "@/domain/types";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/presentation/components/ui/button";
import { useAcceptQuote, useCancelRfq, useInstrument, useTrades, useUnlockThen } from "@/presentation/hooks/queries";
import { useNow } from "@/presentation/hooks/use-now";
import { count, formatAmount } from "@/presentation/lib/format";
import { cn } from "@/presentation/lib/utils";
import { useSession } from "@/presentation/providers/session";
import { Countdown, TimeLeft } from "./countdown";
import { QuoteForm } from "./quote-form";
import { RfqTitle } from "./rfq-list";
import { StatusBadge } from "./status-badge";
import { TradeStatus } from "./trade-status";

function DetailHeader({ rfq, note, right }: { rfq: Rfq; note: string; right: ReactNode }) {
  return (
    // The controls move under the title when the pane is too narrow for both.
    <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2 border-b border-border pb-4">
      <div className="min-w-0 flex-[1_1_14rem]">
        <RfqTitle rfq={rfq} className="text-lg tracking-tight" />
        <p className="mt-0.5 text-sm text-muted-foreground">{note}</p>
      </div>
      <div className="flex flex-none items-center gap-3">{right}</div>
    </div>
  );
}

/** The newest trade of an RFQ that is not a failure, and the newest failure. */
function tradesOf(trades: Trade[] | undefined, rfqId: string): { live?: Trade; failed?: Trade } {
  const mine = (trades ?? []).filter((t) => t.rfqId === rfqId).sort((a, b) => b.createdAt - a.createdAt);
  return { live: mine.find((t) => t.status !== "failed"), failed: mine.find((t) => t.status === "failed") };
}

export function MyRfqDetail({ entry }: { entry: RfqWithQuotes }) {
  const now = useNow();
  const trades = useTrades();
  const cancel = useCancelRfq();
  const { rfq, quotes } = entry;
  const status = rfqRowStatus(entry, now);
  const { live, failed } = tradesOf(trades.data, rfq.id);
  const accepted = quotes.find((q) => q.status === "accepted" || q.status === "won");
  const settlingQuote = accepted ?? (live === undefined ? undefined : quotes.find((q) => q.id === live.quoteId));
  const best = bestQuote(quotes, rfq.direction, now);
  const cancellable = canCancel(entry, now);

  return (
    <div className="space-y-4">
      <DetailHeader
        rfq={rfq}
        note={`${count(rfq.makers.length, "maker", "makers")} asked: ${rfq.makers.join(", ")}`}
        right={
          cancellable ? (
            <>
              <Countdown from={rfq.createdAt} until={rfq.deadline} />
              <Button
                variant="ghost"
                size="sm"
                disabled={cancel.isPending}
                onClick={() =>
                  cancel.mutate(rfq.id, { onSuccess: () => toast("Request cancelled", { description: "The makers' quotes are released." }) })
                }
              >
                Cancel
              </Button>
            </>
          ) : (
            <StatusBadge status={status} />
          )
        }
      />

      {live !== undefined || accepted !== undefined ? (
        <TradeStatus rfq={rfq} quote={settlingQuote} trade={live} role="taker" />
      ) : (
        <>
          {failed !== undefined && (
            <p className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
              Settlement with {failed.maker} failed — nothing moved{failed.reason === undefined ? "." : `: ${failed.reason}`}
              {status === "open" ? " You can accept another quote." : ""}
            </p>
          )}
          <p className="label-caps">Quotes — only you see them</p>
          {quotes.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {status === "open" ? "Waiting for quotes… the makers you asked were notified." : "No quotes came in."}
            </p>
          ) : (
            <div className="space-y-2">
              {quotes.map((q) => (
                <QuoteRow key={q.id} entry={entry} quote={q} best={q.id === best?.id} now={now} />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function QuoteRow({ entry, quote, best, now }: { entry: RfqWithQuotes; quote: Quote; best: boolean; now: number }) {
  const { rfq } = entry;
  const accept = useAcceptQuote();
  const instrument = useInstrument();
  const { notional } = quoteTerms(rfq.qty, quote.price, rfq.feeBps);
  const actionable = canAccept(entry, quote, now);
  const { act, locked, unlocking } = useUnlockThen(() =>
    accept.mutate(quote.id, {
      onSuccess: () =>
        toast.success("Quote accepted", { description: "Your browser signs your side; the trade settles atomically." }),
    }),
  );

  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-x-3 gap-y-2 rounded-md border p-3",
        best ? "border-primary/60 bg-primary/5" : "border-border",
      )}
    >
      <div className="min-w-0 flex-[1_1_9rem]">
        <p className="flex items-center gap-2 text-sm font-medium">
          {quote.maker}
          {best && <Badge className="h-4 px-1.5 text-[0.625rem] uppercase">best</Badge>}
        </p>
        <p className="tabular text-xs text-muted-foreground">
          @ {formatAmount(quote.price)} {rfq.quote} per {rfq.base}
        </p>
        {quote.status === "open" && quote.validUntil > now && <TimeLeft until={quote.validUntil} prefix="valid " />}
      </div>
      <div className="ml-auto flex flex-none items-center gap-3">
        <div className="text-right">
          <p className="text-xs text-muted-foreground">You {rfq.direction === "BUY" ? "pay" : "receive"}</p>
          <p className="tabular font-semibold">
            {formatAmount(notional, instrument(rfq.quote).decimals)} {rfq.quote}
          </p>
        </div>
        {actionable ? (
          <Button
            size="sm"
            disabled={accept.isPending || unlocking}
            title={locked ? "Accepting allocates your funds: unlock signing first" : undefined}
            onClick={act}
          >
            {accept.isPending ? "Accepting…" : locked ? "Unlock to accept" : best ? "Accept best" : "Accept"}
          </Button>
        ) : (
          <StatusBadge status={quote.status === "open" ? "expired" : quote.status} />
        )}
      </div>
    </div>
  );
}

export function IncomingRfqDetail({ entry }: { entry: IncomingRfq }) {
  const now = useNow();
  const { rfq, myQuote } = entry;
  const status = incomingRowStatus(entry, now);
  const quotable = canQuote(entry, now);
  const live = rfq.status === "open" && rfq.deadline > now;

  return (
    <div className="space-y-4">
      <DetailHeader
        rfq={rfq}
        note={`from ${rfq.taker} · desk fee ${rfq.feeBps} bps, paid by the maker who wins`}
        right={
          live && (myQuote === undefined || myQuote.status === "open" || myQuote.status === "allocating") ? (
            <Countdown from={rfq.createdAt} until={rfq.deadline} />
          ) : (
            <StatusBadge status={status === "new" ? "open" : status} />
          )
        }
      />
      {myQuote !== undefined ? (
        <MyQuote rfq={rfq} quote={myQuote} now={now} />
      ) : quotable ? (
        <QuoteForm rfq={rfq} />
      ) : (
        <p className="text-sm text-muted-foreground">
          {status === "expired" ? "This request expired before you quoted." : "This request closed before you quoted."}
        </p>
      )}
    </div>
  );
}

const QUOTE_FATE: Partial<Record<Quote["status"], string>> = {
  allocating: "Committing your funds on the ledger — the taker sees the quote once they are locked.",
  open: "Live: the taker can accept it until it expires. Your funds stay committed until then.",
  lost: "Another quote won. Your funds are released.",
  expired: "Expired before the taker accepted. Your funds are released.",
  cancelled: "The taker cancelled the request. Your funds are released.",
  failed: "This quote could not be placed: committing your funds failed (insufficient funds?).",
};

function MyQuote({ rfq, quote, now }: { rfq: Rfq; quote: Quote; now: number }) {
  const trades = useTrades();
  const instrument = useInstrument();
  const { locked } = useSession();
  const trade = trades.data?.find((t) => t.quoteId === quote.id);
  const terms = makerTerms(rfq, quote.price);
  const dq = instrument(rfq.quote).decimals;
  const status = quote.status === "open" && quote.validUntil <= now ? "expired" : quote.status;
  const settlingOrDone = status === "accepted" || status === "won" || trade !== undefined;

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <p className="label-caps">Your quote</p>
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 rounded-md border border-border p-3">
          <div className="min-w-0 flex-[1_1_12rem] space-y-0.5">
            <p className="tabular font-semibold">
              {formatAmount(quote.price)} {rfq.quote} per {rfq.base}
            </p>
            <p className="tabular text-xs text-muted-foreground">
              If you win you {terms.deliversBase ? "deliver" : "receive"}{" "}
              {formatAmount(rfq.qty, instrument(rfq.base).decimals)} {rfq.base} and{" "}
              {terms.deliversBase ? "receive" : "pay"} {formatAmount(terms.quoteTotal, dq)} {rfq.quote},{" "}
              {terms.deliversBase ? "net of" : "including"} the {formatAmount(terms.fee, dq)} {rfq.quote} desk fee
            </p>
            {status === "open" && <TimeLeft until={quote.validUntil} prefix="valid " />}
          </div>
          <StatusBadge status={status} className="ml-auto" />
        </div>
        {!settlingOrDone && QUOTE_FATE[status] !== undefined && (
          <p className="text-sm text-muted-foreground">
            {QUOTE_FATE[status]}
            {status === "allocating" && locked ? " Unlock signing to commit them." : ""}
          </p>
        )}
      </div>
      {settlingOrDone && <TradeStatus rfq={rfq} quote={quote} trade={trade} role="maker" />}
    </div>
  );
}
