import { Plus, Search } from "lucide-react";
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

const sideClass = (tint: "success" | "destructive"): string =>
  cn(
    "rounded-lg border border-border py-2.5 text-sm font-semibold text-muted-foreground transition-colors hover:border-foreground/30 hover:text-foreground data-checked:text-foreground",
    tint === "success"
      ? "data-checked:border-success/50 data-checked:bg-success/15"
      : "data-checked:border-destructive/50 data-checked:bg-destructive/15",
  );

const MAKER_SEARCH_FROM = 8;

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
  const [makerQuery, setMakerQuery] = useState("");

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
  const chosen = picked.filter((hint) => candidates.some((m) => m.hint === hint));
  const allChosen = candidates.length > 0 && candidates.every((m) => chosen.includes(m.hint));
  const capped = candidates.length > RFQ_MAKERS_MAX;
  const full = chosen.length >= RFQ_MAKERS_MAX;
  const needle = makerQuery.trim().toLowerCase();
  const listed =
    needle === ""
      ? candidates
      : candidates.filter((m) => `${m.displayName} ${m.hint}`.toLowerCase().includes(needle));
  const sell = side === "SELL";
  const valid =
    isPositiveDecimal(qty) &&
    baseSym !== undefined &&
    quoteSym !== undefined &&
    chosen.length > 0 &&
    chosen.length <= RFQ_MAKERS_MAX;

  const pickBase = (symbol: string): void => {
    if (symbol === quoteSym) setQuote(baseSym);
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
        if (v) {
          setMakerQuery("");
          void makers.refetch();
        }
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
          className="min-w-0 space-y-4"
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
            <div className="flex items-center justify-between gap-3">
              <span className="label-caps">Makers to ask</span>
              <span className="flex items-center gap-3 text-xs">
                {capped && (
                  <span className="tabular text-muted-foreground">
                    {chosen.length === 0 ? `up to ${RFQ_MAKERS_MAX}` : `${chosen.length} of ${RFQ_MAKERS_MAX}`}
                  </span>
                )}
                {(capped ? chosen.length > 0 : candidates.length > 1) && (
                  <button
                    type="button"
                    className="text-primary underline-offset-2 hover:underline"
                    onClick={() => setPicked(capped || allChosen ? [] : candidates.map((m) => m.hint))}
                  >
                    {capped || allChosen ? "Clear" : "Select all"}
                  </button>
                )}
              </span>
            </div>
            {makers.isPending ? (
              <p className="text-sm text-muted-foreground">Loading makers…</p>
            ) : candidates.length === 0 ? (
              <p className="text-sm text-muted-foreground">No makers have maker mode on right now.</p>
            ) : (
              <div className="overflow-hidden rounded-lg border border-border">
                {candidates.length >= MAKER_SEARCH_FROM && (
                  <label className="flex items-center gap-2 border-b border-border px-3">
                    <Search className="size-3.5 flex-none text-muted-foreground" />
                    <input
                      value={makerQuery}
                      onChange={(e) => setMakerQuery(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") e.preventDefault();
                      }}
                      placeholder={`Search ${candidates.length} makers`}
                      aria-label="Search makers"
                      autoComplete="off"
                      className="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground/60"
                    />
                  </label>
                )}
                <ul className="relative max-h-[9.25rem] overflow-y-auto overscroll-contain py-1">
                  {listed.map((m) => {
                    const on = chosen.includes(m.hint);
                    const blocked = !on && full;
                    return (
                      <li key={m.hint}>
                        <label
                          className={cn(
                            "flex items-center gap-2.5 px-3 py-1.5 text-sm transition-colors",
                            blocked ? "text-muted-foreground" : "hover:bg-accent",
                            on && "bg-primary/5",
                          )}
                        >
                          <Checkbox
                            checked={on}
                            disabled={blocked}
                            onCheckedChange={(v) =>
                              setPicked((cur) => (v === true ? [...cur, m.hint] : cur.filter((h) => h !== m.hint)))
                            }
                          />
                          <span className="max-w-[70%] flex-none truncate">{m.displayName}</span>
                          {m.displayName !== m.hint && (
                            <span
                              className="tabular ml-auto truncate pl-2 text-xs text-muted-foreground"
                              title={m.hint}
                            >
                              {m.hint}
                            </span>
                          )}
                        </label>
                      </li>
                    );
                  })}
                  {listed.length === 0 && (
                    <li className="px-3 py-1.5 text-sm text-muted-foreground">
                      No maker matches &ldquo;{makerQuery.trim()}&rdquo;.
                    </li>
                  )}
                </ul>
              </div>
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
