import { Loader2, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/presentation/components/ui/button";
import { Card, CardContent } from "@/presentation/components/ui/card";
import { useFaucet, useNetwork, useUnlockThen } from "@/presentation/hooks/queries";

export function FaucetCard() {
  const faucet = useFaucet();
  const drips = useNetwork().data?.faucet ?? [];

  const { act, locked, unlocking } = useUnlockThen(() =>
    faucet.mutate(undefined, {
      onSuccess: (actions) =>
        toast.success("Test tokens on the way", {
          description:
            actions.length > 0
              ? `Your browser accepts ${actions.length === 1 ? "the transfer" : `all ${actions.length} transfers`} — balances update in a moment.`
              : "Balances update in a moment.",
        }),
    }),
  );

  if (drips.length === 0) return null;

  return (
    <Card className="bg-primary/5 ring-primary/40">
      <CardContent className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
        <div className="min-w-0 space-y-1">
          <p className="font-heading text-base leading-snug font-medium">Test tokens</p>
          <p className="text-sm text-muted-foreground">
            Get {drips.map((d) => `${d.amount} ${d.symbol}`).join(", ")} from the desk's DevNet faucet.
          </p>
        </div>
        <Button
          size="lg"
          disabled={faucet.isPending || unlocking}
          onClick={act}
        >
          {faucet.isPending ? (
            <>
              <Loader2 className="animate-spin" /> Sending…
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
