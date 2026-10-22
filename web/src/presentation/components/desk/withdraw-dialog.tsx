/**
 * Send holdings to any Canton party: the receiver is a full party id, and gets
 * a transfer offer to accept in their own wallet.
 */
import { Loader2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { cmpDecimal, isPositiveDecimal } from "@/domain/money";
import type { Balance } from "@/domain/types";
import { Button } from "@/presentation/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/presentation/components/ui/dialog";
import { Input } from "@/presentation/components/ui/input";
import { Label } from "@/presentation/components/ui/label";
import { useInstrument, useUnlockThen, useWithdraw } from "@/presentation/hooks/queries";
import { sanitizeDecimal } from "@/presentation/lib/amount";
import { formatAmount } from "@/presentation/lib/format";
import { useSession } from "@/presentation/providers/session";
import { TokenIcon } from "./token-icon";

/** Mount it keyed by the symbol, so each open starts with an empty form. */
export function WithdrawDialog({ balance, onClose }: { balance: Balance | null; onClose: () => void }) {
  const withdraw = useWithdraw();
  const { me } = useSession();
  const instrument = useInstrument();
  const [amount, setAmount] = useState("");
  const [receiver, setReceiver] = useState("");

  const target = receiver.trim();
  const amountOk = balance !== null && isPositiveDecimal(amount) && cmpDecimal(amount, balance.available) <= 0;
  const receiverOk = /^.+::.+$/.test(target);
  const toSelf = me !== null && target === me.partyId;
  const valid = amountOk && receiverOk && !toSelf;

  const { act, locked, unlocking } = useUnlockThen(() => {
    if (balance === null) return;
    withdraw.mutate(
      { symbol: balance.symbol, amount, receiver: target },
      {
        onSuccess: () => {
          onClose();
          toast.success("Withdrawal on its way", {
            description: "Your browser signs it; the receiver accepts the transfer in their wallet.",
          });
        },
      },
    );
  });

  return (
    <Dialog open={balance !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2.5">
            {balance !== null && <TokenIcon symbol={balance.symbol} />}
            Withdraw {balance?.symbol}
          </DialogTitle>
          <DialogDescription>
            The receiver gets a transfer offer and accepts it in their own wallet.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (valid) act();
          }}
        >
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="withdraw-amount">Amount</Label>
              {balance !== null && (
                <button
                  type="button"
                  className="tabular text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
                  onClick={() => setAmount(balance.available)}
                >
                  Available: {formatAmount(balance.available, instrument(balance.symbol).decimals)} {balance.symbol}
                </button>
              )}
            </div>
            <Input
              id="withdraw-amount"
              className="tabular"
              inputMode="decimal"
              autoComplete="off"
              placeholder="0.0"
              value={amount}
              onChange={(e) => setAmount(sanitizeDecimal(e.target.value))}
              aria-invalid={amount !== "" && !amountOk ? true : undefined}
            />
            {amount !== "" && balance !== null && isPositiveDecimal(amount) && !amountOk && (
              <p className="text-xs text-destructive">More than you have available.</p>
            )}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="withdraw-receiver">Receiver party id</Label>
            <Input
              id="withdraw-receiver"
              className="tabular text-xs"
              placeholder="party::1220…"
              autoComplete="off"
              spellCheck={false}
              value={receiver}
              onChange={(e) => setReceiver(e.target.value)}
              aria-invalid={target !== "" && (!receiverOk || toSelf) ? true : undefined}
            />
            {target !== "" && !receiverOk && (
              <p className="text-xs text-destructive">A full party id looks like hint::fingerprint.</p>
            )}
            {toSelf && <p className="text-xs text-destructive">That is your own party id.</p>}
          </div>
          <Button type="submit" className="w-full" disabled={!valid || withdraw.isPending || unlocking}>
            {withdraw.isPending ? (
              <>
                <Loader2 className="animate-spin" /> Sending…
              </>
            ) : locked ? (
              "Unlock to withdraw"
            ) : (
              "Withdraw"
            )}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}
