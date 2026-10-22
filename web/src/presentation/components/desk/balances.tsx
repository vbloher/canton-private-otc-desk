/** Holdings per instrument: what is free to trade and what quotes and trades have committed. */
import type { Balance } from "@/domain/types";
import { isPositiveDecimal } from "@/domain/money";
import { Button } from "@/presentation/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/presentation/components/ui/card";
import { Skeleton } from "@/presentation/components/ui/skeleton";
import { useBalances, useInstrument } from "@/presentation/hooks/queries";
import { formatAmount } from "@/presentation/lib/format";
import { Notice } from "./notice";
import { TokenIcon } from "./token-icon";

export function Balances({ onWithdraw }: { onWithdraw: (balance: Balance) => void }) {
  const balances = useBalances();
  const instrument = useInstrument();

  return (
    <Card>
      <CardHeader>
        <CardTitle>Balances</CardTitle>
      </CardHeader>
      <CardContent>
        {balances.data !== undefined ? (
          balances.data.length === 0 ? (
            <Notice>No instruments on this desk yet.</Notice>
          ) : (
            <div className="divide-y divide-border">
              {balances.data.map((b) => {
                const i = instrument(b.symbol);
                return (
                  <div key={b.symbol} className="flex items-center gap-3 py-3 first:pt-0 last:pb-0">
                    <TokenIcon symbol={b.symbol} />
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium">{b.symbol}</p>
                      <p className="truncate text-xs text-muted-foreground">{i.name}</p>
                    </div>
                    <div className="text-right">
                      <p className="tabular text-sm font-semibold" title={`${b.available} ${b.symbol} available`}>
                        {formatAmount(b.available, i.decimals)}
                      </p>
                      {isPositiveDecimal(b.locked) && (
                        <p className="tabular text-xs text-muted-foreground" title={`${b.locked} ${b.symbol} locked`}>
                          + {formatAmount(b.locked, i.decimals)} locked
                        </p>
                      )}
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={!isPositiveDecimal(b.available)}
                      onClick={() => onWithdraw(b)}
                    >
                      Withdraw
                    </Button>
                  </div>
                );
              })}
            </div>
          )
        ) : balances.isError ? (
          <Notice>
            <p>Could not read your balances.</p>
            <Button variant="outline" size="sm" disabled={balances.isFetching} onClick={() => void balances.refetch()}>
              Try again
            </Button>
          </Notice>
        ) : (
          <div className="space-y-2">
            <Skeleton className="h-12" />
            <Skeleton className="h-12" />
            <Skeleton className="h-12" />
          </div>
        )}
      </CardContent>
    </Card>
  );
}
