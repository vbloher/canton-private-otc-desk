import { useId, useState } from "react";
import { toast } from "sonner";
import { cmpDecimal, isPositiveDecimal } from "@/domain/money";
import { makerTerms, QUOTE_TTL_CHOICES_S, QUOTE_TTL_DEFAULT_S } from "@/domain/rfq";
import type { Rfq } from "@/domain/types";
import { Button } from "@/presentation/components/ui/button";
import { Input } from "@/presentation/components/ui/input";
import { Label } from "@/presentation/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/presentation/components/ui/radio-group";
import { useBalances, useInstrument, useSubmitQuote, useUnlockThen } from "@/presentation/hooks/queries";
import { useNow } from "@/presentation/hooks/use-now";
import { sanitizeDecimal } from "@/presentation/lib/amount";
import { formatAmount, formatTtl } from "@/presentation/lib/format";
import { DetailRow } from "./detail-row";

export function QuoteForm({ rfq }: { rfq: Rfq }) {
  const [price, setPrice] = useState("");
  const [ttl, setTtl] = useState<number>(QUOTE_TTL_DEFAULT_S);
  const submit = useSubmitQuote();
  const balances = useBalances();
  const instrument = useInstrument();
  const now = useNow();
  const ttlLabel = useId();

  const priced = isPositiveDecimal(price);
  const terms = priced ? makerTerms(rfq, price) : undefined;
  const dust = terms !== undefined && !isPositiveDecimal(terms.notional);
  const leftS = Math.floor((rfq.deadline - now) / 1000);
  const db = instrument(rfq.base).decimals;
  const dq = instrument(rfq.quote).decimals;
  const show = (value: string | undefined, symbol: string, digits: number): string =>
    value === undefined ? "—" : `${formatAmount(value, digits)} ${symbol}`;

  const short = (symbol: string, need: string): boolean => {
    const have = balances.data?.find((b) => b.symbol === symbol)?.available;
    return isPositiveDecimal(need) && have !== undefined && cmpDecimal(have, need) < 0;
  };
  const shortOf =
    terms === undefined
      ? undefined
      : short(rfq.base, terms.commitsBase)
        ? rfq.base
        : short(rfq.quote, terms.commitsQuote)
          ? rfq.quote
          : undefined;

  const send = (): void =>
    submit.mutate(
      { rfqId: rfq.id, quote: { price, ttlSeconds: ttl } },
      { onSuccess: () => toast.success("Quote sent", { description: "Committing your funds on the ledger…" }) },
    );
  const { act, locked, unlocking } = useUnlockThen(send);

  const delivers = rfq.direction === "BUY";

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (priced && !dust) act();
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor="quote-price">Your price</Label>
        <div className="flex items-stretch gap-2">
          <Input
            id="quote-price"
            value={price}
            onChange={(e) => setPrice(sanitizeDecimal(e.target.value))}
            className="tabular h-12 flex-1 text-lg"
            inputMode="decimal"
            autoComplete="off"
            placeholder="0.00"
          />
          <span className="flex items-center rounded-lg border border-input bg-secondary/60 px-3 text-sm font-medium">
            {rfq.quote} / {rfq.base}
          </span>
        </div>
        <p className="text-xs text-muted-foreground">
          {rfq.quote} per 1 {rfq.base}
        </p>
      </div>

      <div className="space-y-1.5">
        <Label id={ttlLabel}>Quote stands for</Label>
        <RadioGroup
          value={String(ttl)}
          onValueChange={(v) => setTtl(Number(v))}
          aria-labelledby={ttlLabel}
          className="flex-wrap items-center gap-1.5"
        >
          {QUOTE_TTL_CHOICES_S.map((seconds) => (
            <RadioGroupItem
              key={seconds}
              value={String(seconds)}
              className="rounded-md px-2.5 py-1 text-sm font-medium text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground data-checked:bg-primary/15 data-checked:text-foreground data-checked:ring-1 data-checked:ring-primary/40 data-checked:ring-inset"
            >
              {formatTtl(seconds)}
            </RadioGroupItem>
          ))}
        </RadioGroup>
        {ttl > leftS && leftS > 0 && (
          <p className="text-xs text-muted-foreground">The request closes sooner: the quote ends with it.</p>
        )}
      </div>

      <div className="rounded-lg bg-secondary p-4">
        <p className="label-caps">If you win</p>
        <div className="mt-3 space-y-1.5 text-sm">
          {delivers ? (
            <>
              <DetailRow label="You deliver" value={show(rfq.qty, rfq.base, db)} />
              <DetailRow label="You receive" value={show(terms?.notional, rfq.quote, dq)} />
            </>
          ) : (
            <>
              <DetailRow label="You receive" value={show(rfq.qty, rfq.base, db)} />
              <DetailRow label="You pay" value={show(terms?.notional, rfq.quote, dq)} />
            </>
          )}
          <DetailRow
            label={`Desk fee (${rfq.feeBps} bps)`}
            value={terms === undefined ? "—" : `${delivers ? "−" : "+"} ${show(terms.fee, rfq.quote, dq)}`}
          />
        </div>
        <div className="mt-3 flex items-baseline justify-between border-t border-border pt-3">
          <span className="text-sm font-medium">{delivers ? "You receive after the fee" : "Total you pay"}</span>
          <span className="tabular text-xl font-semibold">{show(terms?.quoteTotal, rfq.quote, dq)}</span>
        </div>
      </div>

      {dust && <p className="text-sm text-destructive">At this price the {rfq.quote} leg rounds to zero — raise it.</p>}
      {shortOf !== undefined && (
        <p className="text-sm text-destructive">Your available {shortOf} does not cover what this quote commits.</p>
      )}
      <p className="text-sm text-muted-foreground">
        Only the requester sees your price. Sending it commits your funds until the quote wins, loses or expires.
      </p>
      <Button type="submit" className="w-full" disabled={!priced || dust || submit.isPending || unlocking}>
        {submit.isPending ? "Sending…" : locked ? "Unlock to send quote" : "Send quote"}
      </Button>
    </form>
  );
}
