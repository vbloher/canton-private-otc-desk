/**
 * New request for quotes, ticket style: pick a side, the token the trade is
 * about and what it settles in, the amount, the makers to ask and how long
 * they have. Only the makers picked ever see the request.
 */
import { Plus } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { isPositiveDecimal } from "@/domain/money";
import { RFQ_MAKERS_MAX, RFQ_TTL_CHOICES_S, RFQ_TTL_DEFAULT_S } from "@/domain/rfq";
import type { Direction } from "@/domain/types";
import { Button } from "@/presentation/components/ui/button";
import { Checkbox } from "@/presentation/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/presentation/components/ui/dialog";
import { RadioGroup, RadioGroupItem } from "@/presentation/components/ui/radio-group";
import { useBalances, useCreateRfq, useDeskTerms, useInstruments, useMakers } from "@/presentation/hooks/queries";
import { sanitizeDecimal } from "@/presentation/lib/amount";
import { count, formatAmount, formatTtl } from "@/presentation/lib/format";
import { cn } from "@/presentation/lib/utils";
import { TokenChip } from "./token-chip";

/** Buy / Sell: the direction tint is the only colour in the ticket besides amber. */
const sideClass = (tint: "success" | "destructive"): string =>
  cn(
    "rounded-lg border border-border py-2.5 text-sm font-semibold text-muted-foreground transition-colors hover:border-foreground/30 hover:text-foreground data-checked:text-foreground",
    tint === "success"
      ? "data-checked:border-success/50 data-checked:bg-success/15"
      : "data-checked:border-destructive/50 data-checked:bg-destructive/15",
  );

const pillClass =
  "rounded-full border border-border px-3 py-1 text-xs text-muted-foreground transition-colors hover:border-foreground/30 hover:text-foreground data-checked:border-primary data-checked:bg-primary/15 data-checked:font-medium data-checked:text-foreground";

export function NewRfqDialog() {
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [side, setSide] = useState<Direction>("BUY");
  const [base, setBase] = useState<string>();
  const [quote, setQuote] = useState<string>();
  const [qty, setQty] = useState("");
  const [ttl, setTtl] = useState<number>(RFQ_TTL_DEFAULT_S);
  const [picked, setPicked] = useState<string[]>([]);

  const instruments = useInstruments();
  const makers = useMakers(open);
  const balances = useBalances(open);
  const desk = useDeskTerms();
  const createRfq = useCreateRfq();

  const list = instruments.data ?? [];
  const baseSym = base ?? list[0]?.symbol;
  const quoteSym = quote !== undefined && quote !== baseSym ? quote : list.find((i) => i.symbol !== baseSym)?.symbol;
  const decimalsOf = (symbol: string | undefined): number => list.find((i) => i.symbol === symbol)?.decimals ?? 10;
  const available = (symbol: string | undefined): string | undefined =>
    balances.data?.find((b) => b.symbol === symbol)?.available;

  const candidates = makers.data ?? [];
  // A maker who switched maker mode off since being picked is no longer offered.
  const chosen = picked.filter((hint) => candidates.some((m) => m.hint === hint));
  const allChosen = candidates.length > 0 && candidates.every((m) => chosen.includes(m.hint));
  const sell = side === "SELL";
  const valid =
    isPositiveDecimal(qty) &&
    baseSym !== undefined &&
    quoteSym !== undefined &&
    chosen.length > 0 &&
    chosen.length <= RFQ_MAKERS_MAX;

  const pickBase = (symbol: string): void => {
    if (symbol === quoteSym) setQuote(baseSym); // the legs swap rather than collide
    setBase(symbol);
  };

  const submit = (): void => {
    if (!valid) return;
    createRfq.mutate(
      { direction: side, qty, base: baseSym, quote: quoteSym, makers: chosen, ttlSeconds: ttl },
      {
        onSuccess: (rfq) => {
          setOpen(false);
          setQty("");
          setPicked([]);
          toast.success("Request sent", { description: `${count(rfq.makers.length, "maker", "makers")} can quote now.` });
          void navigate(`/trade?tab=mine&rfq=${rfq.id}`);
        },
      },
    );
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        // maker-mode changes raise no event: refresh the list whenever the ticket opens
        if (v) void makers.refetch();
      }}
    >
      <DialogTrigger asChild>
        <Button size="lg" className="w-full font-semibold">
          <Plus />
          Request quotes
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Request quotes</DialogTitle>
          <DialogDescription>Only the makers you pick see this request — or how it ends.</DialogDescription>
        </DialogHeader>

        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <div>
            <span className="label-caps">I want to</span>
            <RadioGroup
              value={side}
              onValueChange={(v) => setSide(v === "SELL" ? "SELL" : "BUY")}
              aria-label="Side"
              className="mt-1.5 grid grid-cols-2 gap-1.5"
            >
              <RadioGroupItem value="BUY" className={sideClass("success")}>
                Buy
              </RadioGroupItem>
              <RadioGroupItem value="SELL" className={sideClass("destructive")}>
                Sell
              </RadioGroupItem>
            </RadioGroup>
          </div>

          <div className="rounded-lg bg-secondary p-4">
            <div className="flex items-center justify-between">
              <span className="label-caps">{sell ? "Sell" : "Buy"}</span>
              {sell && baseSym !== undefined && available(baseSym) !== undefined && (
                <button
                  type="button"
                  className="tabular text-xs text-muted-foreground hover:text-foreground"
                  title="Sell everything available"
                  onClick={() => {
                    const all = available(baseSym);
                    if (all !== undefined && isPositiveDecimal(all)) setQty(all);
                  }}
                >
                  Available: {formatAmount(available(baseSym) ?? "0", decimalsOf(baseSym))}
                </button>
              )}
            </div>
            <RadioGroup
              value={baseSym ?? ""}
              onValueChange={pickBase}
              aria-label={sell ? "Token to sell" : "Token to buy"}
              className="mt-2 flex-wrap gap-1.5"
            >
              {list.map((i) => (
                <TokenChip key={i.symbol} symbol={i.symbol} />
              ))}
            </RadioGroup>
            <input
              value={qty}
              onChange={(e) => setQty(sanitizeDecimal(e.target.value))}
              placeholder="0"
              inputMode="decimal"
              autoComplete="off"
              aria-label={`Amount to ${sell ? "sell" : "buy"}`}
              className="tabular mt-2 w-full bg-transparent text-2xl font-semibold outline-none placeholder:text-muted-foreground/40"
            />
          </div>

          <div className="rounded-lg bg-secondary p-4">
            <div className="flex items-center justify-between">
              <span className="label-caps">{sell ? "Receiving" : "Paying with"}</span>
              {!sell && quoteSym !== undefined && available(quoteSym) !== undefined && (
                <span className="tabular text-xs text-muted-foreground">
                  Available: {formatAmount(available(quoteSym) ?? "0", decimalsOf(quoteSym))}
                </span>
              )}
            </div>
            <RadioGroup
              value={quoteSym ?? ""}
              onValueChange={setQuote}
              aria-label={sell ? "Token to receive" : "Token to pay with"}
              className="mt-2 flex-wrap gap-1.5"
            >
              {list
                .filter((i) => i.symbol !== baseSym)
                .map((i) => (
                  <TokenChip key={i.symbol} symbol={i.symbol} />
                ))}
            </RadioGroup>
          </div>

          <div className="space-y-1.5">
            <span className="label-caps">Quote window</span>
            <RadioGroup
              value={String(ttl)}
              onValueChange={(v) => setTtl(Number(v))}
              aria-label="Quote window"
              className="flex-wrap gap-1"
            >
              {RFQ_TTL_CHOICES_S.map((seconds) => (
                <RadioGroupItem key={seconds} value={String(seconds)} className={pillClass}>
                  {formatTtl(seconds)}
                </RadioGroupItem>
              ))}
            </RadioGroup>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="label-caps">Makers to ask</span>
              {candidates.length > 1 && (
                <button
                  type="button"
                  className="text-xs text-primary underline-offset-2 hover:underline"
                  onClick={() => setPicked(allChosen ? [] : candidates.slice(0, RFQ_MAKERS_MAX).map((m) => m.hint))}
                >
                  {allChosen ? "Clear" : "Select all"}
                </button>
              )}
            </div>
            {makers.isPending ? (
              <p className="text-sm text-muted-foreground">Loading makers…</p>
            ) : candidates.length === 0 ? (
              <p className="text-sm text-muted-foreground">No makers have maker mode on right now.</p>
            ) : (
              <div className="-my-2 max-h-40 space-y-1.5 overflow-y-auto py-2">
                {candidates.map((m) => (
                  <label key={m.hint} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={chosen.includes(m.hint)}
                      onCheckedChange={(v) =>
                        setPicked((cur) => (v === true ? [...cur, m.hint] : cur.filter((h) => h !== m.hint)))
                      }
                    />
                    <span>{m.displayName}</span>
                    {m.displayName !== m.hint && <span className="tabular text-xs text-muted-foreground">{m.hint}</span>}
                  </label>
                ))}
              </div>
            )}
            {chosen.length > RFQ_MAKERS_MAX && (
              <p className="text-xs text-destructive">Ask at most {RFQ_MAKERS_MAX} makers.</p>
            )}
          </div>

          <Button type="submit" className="w-full" disabled={!valid || createRfq.isPending}>
            {createRfq.isPending ? "Sending…" : "Request quotes"}
          </Button>
          <p className="text-center text-xs text-muted-foreground">
            No desk fee for takers
            {desk.data !== undefined ? ` — the winning maker pays ${desk.data.feeBps} bps` : ""}. You pay only the
            price you accept.
          </p>
        </form>
      </DialogContent>
    </Dialog>
  );
}
