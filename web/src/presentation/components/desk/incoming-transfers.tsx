/** Transfers offered to this wallet; accepting one is a ledger action the browser signs. */
import { Loader2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import type { IncomingTransfer } from "@/domain/types";
import { Button } from "@/presentation/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/presentation/components/ui/card";
import { useAcceptIncoming, useIncomingTransfers, useInstrument, useUnlockThen } from "@/presentation/hooks/queries";
import { formatAmount } from "@/presentation/lib/format";
import { useSignDropped } from "@/presentation/providers/auto-signer";

export function IncomingTransfers() {
  const incoming = useIncomingTransfers();
  const transfers = incoming.data ?? [];
  return (
    <Card>
      <CardHeader>
        <CardTitle>Incoming transfers</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        {transfers.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {incoming.isPending ? "Looking for transfers…" : "Nothing waiting. Transfers sent to your party id show up here."}
          </p>
        ) : (
          transfers.map((t) => <TransferRow key={t.contractId} transfer={t} />)
        )}
      </CardContent>
    </Card>
  );
}

/** Each row owns its mutation, so several transfers can be accepted at once. */
function TransferRow({ transfer }: { transfer: IncomingTransfer }) {
  const accept = useAcceptIncoming();
  const instrument = useInstrument();
  // The POST only queues the signature: the row stays busy until the inbox stops listing the
  // transfer, or until its action fails, expires or disappears (then it can be accepted again).
  const [queued, setQueued] = useState<readonly string[] | null>(null);
  useSignDropped((actionId) => {
    if (queued?.includes(actionId) === true) setQueued(null);
  });
  const busy = accept.isPending || queued !== null;

  const { act, locked, unlocking } = useUnlockThen(() =>
    accept.mutate(transfer.contractId, {
      onSuccess: (actions) => {
        if (actions.length > 0) setQueued(actions.map((a) => a.id));
        toast("Accepting the transfer", { description: "Your browser signs it — the balance updates in a moment." });
      },
    }),
  );

  return (
    <div className="flex items-center justify-between gap-3 rounded-md border border-primary/40 bg-primary/5 p-3">
      <div>
        <p className="tabular text-sm font-medium" title={`${transfer.amount} ${transfer.symbol}`}>
          + {formatAmount(transfer.amount, instrument(transfer.symbol).decimals)} {transfer.symbol}
        </p>
        <p className="text-xs text-muted-foreground">from {transfer.sender}</p>
      </div>
      <Button size="sm" disabled={busy || unlocking} onClick={act}>
        {busy ? (
          <>
            <Loader2 className="animate-spin" /> Accepting…
          </>
        ) : locked ? (
          "Unlock to accept"
        ) : (
          "Accept"
        )}
      </Button>
    </div>
  );
}
