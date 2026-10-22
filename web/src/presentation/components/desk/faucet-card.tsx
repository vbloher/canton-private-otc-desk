/**
 * Test tokens on the local sandbox: the desk mints an offer per instrument and
 * this browser accepts each one (a signature apiece, done by the auto-signer).
 */
import { Loader2, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/presentation/components/ui/button";
import { Card, CardContent } from "@/presentation/components/ui/card";
import { useFaucet, useInstruments, useUnlockThen } from "@/presentation/hooks/queries";

export function FaucetCard() {
  const faucet = useFaucet();
  const symbols = (useInstruments().data ?? []).map((i) => i.symbol);

  const { act, locked, unlocking } = useUnlockThen(() =>
    faucet.mutate(undefined, {
      onSuccess: (actions) =>
        toast.success("Test tokens on the way", {
          description:
            actions.length > 0
              ? `Your browser accepts ${actions.length === 1 ? "the mint" : `all ${actions.length} mints`} — balances update in a moment.`
              : "Balances update in a moment.",
        }),
    }),
  );

  return (
    <Card className="bg-primary/5 ring-primary/40">
      <CardContent className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
        <div className="min-w-0 space-y-1">
          <p className="font-heading text-base leading-snug font-medium">Test tokens</p>
          <p className="text-sm text-muted-foreground">
            Mint {symbols.length > 0 ? symbols.join(", ") : "test tokens"} to your party — local sandbox only.
          </p>
        </div>
        <Button
          size="lg"
          disabled={faucet.isPending || unlocking}
          onClick={act}
        >
          {faucet.isPending ? (
            <>
              <Loader2 className="animate-spin" /> Minting…
            </>
          ) : locked ? (
            "Unlock to get test tokens"
          ) : (
            <>
              <Sparkles /> Get test tokens
            </>
          )}
        </Button>
      </CardContent>
    </Card>
  );
}
