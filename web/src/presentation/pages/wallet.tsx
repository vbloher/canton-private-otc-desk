import { useState } from "react";
import { Balances } from "@/presentation/components/desk/balances";
import { DepositCard } from "@/presentation/components/desk/deposit-card";
import { FaucetCard } from "@/presentation/components/desk/faucet-card";
import { IncomingTransfers } from "@/presentation/components/desk/incoming-transfers";
import { WithdrawDialog } from "@/presentation/components/desk/withdraw-dialog";
import { useBalances } from "@/presentation/hooks/queries";

export function WalletPage() {
  const [withdrawing, setWithdrawing] = useState<string | null>(null);
  const balances = useBalances();
  const balance = withdrawing === null ? null : (balances.data?.find((b) => b.symbol === withdrawing) ?? null);

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6">
      <FaucetCard />
      <Balances onWithdraw={(b) => setWithdrawing(b.symbol)} />
      <IncomingTransfers />
      <DepositCard />
      <WithdrawDialog key={withdrawing ?? "closed"} balance={balance} onClose={() => setWithdrawing(null)} />
    </div>
  );
}
