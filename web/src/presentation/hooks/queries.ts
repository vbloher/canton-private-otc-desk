/**
 * TanStack Query hooks over the application's ports. Reads are keyed by
 * application/query-keys (server events invalidate them); every mutation's
 * error is toasted by the query provider, and the ones that queue ledger
 * actions hand those to the auto-signer straight away.
 */
import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";
import { queryKeys } from "@/application/query-keys";
import type { Instrument, Me, NewQuote, NewRfq, SignAction, Withdrawal } from "@/domain/types";
import { useServices } from "@/presentation/providers/services";
import { useSession } from "@/presentation/providers/session";
import { useSigner } from "@/presentation/providers/signer";

/** Reads that need a session: off while signed out, so nothing 401s. */
function useSignedIn(): boolean {
  return useSession().status === "signed-in";
}

export function useInstruments() {
  const { wallet } = useServices();
  return useQuery({
    queryKey: queryKeys.instruments,
    queryFn: () => wallet.instruments(),
    enabled: useSignedIn(),
    staleTime: 5 * 60_000,
  });
}

/** Instrument by symbol; an unknown symbol gets 10 display digits and its own name. */
export function useInstrument(): (symbol: string) => Instrument {
  const { data } = useInstruments();
  return useMemo(() => {
    const bySymbol = new Map((data ?? []).map((i) => [i.symbol, i]));
    return (symbol: string) => bySymbol.get(symbol) ?? { symbol, name: symbol, decimals: 10 };
  }, [data]);
}

export function useDeskTerms() {
  const { desk } = useServices();
  return useQuery({ queryKey: queryKeys.desk, queryFn: () => desk.desk(), enabled: useSignedIn(), staleTime: 60_000 });
}

export function useMakers(enabled: boolean) {
  const { auth } = useServices();
  const signedIn = useSignedIn();
  return useQuery({ queryKey: queryKeys.makers, queryFn: () => auth.makers(), enabled: enabled && signedIn });
}

export function useMyRfqs() {
  const { desk } = useServices();
  return useQuery({ queryKey: queryKeys.myRfqs, queryFn: () => desk.myRfqs(), enabled: useSignedIn() });
}

export function useIncomingRfqs() {
  const { desk } = useServices();
  return useQuery({ queryKey: queryKeys.incomingRfqs, queryFn: () => desk.incomingRfqs(), enabled: useSignedIn() });
}

export function useRfqHistory() {
  const { desk } = useServices();
  return useQuery({ queryKey: queryKeys.rfqHistory, queryFn: () => desk.rfqHistory(), enabled: useSignedIn() });
}

export function useTrades() {
  const { desk } = useServices();
  return useQuery({ queryKey: queryKeys.trades, queryFn: () => desk.trades(), enabled: useSignedIn() });
}

/**
 * Polled as well: an executed action is acknowledged before the ledger commits
 * it, so the refetch an event triggers can still read the old holdings.
 */
export function useBalances(enabled = true) {
  const { wallet } = useServices();
  const signedIn = useSignedIn();
  return useQuery({
    queryKey: queryKeys.balances,
    queryFn: () => wallet.balances(),
    enabled: enabled && signedIn,
    refetchInterval: 10_000,
    meta: { quiet: true }, // a down desk must not toast every 10 s
  });
}

/** Polled: a transfer someone sends here raises no event for this party. */
export function useIncomingTransfers() {
  const { wallet } = useServices();
  return useQuery({
    queryKey: queryKeys.incomingTransfers,
    queryFn: () => wallet.incoming(),
    enabled: useSignedIn(),
    refetchInterval: 10_000,
    meta: { quiet: true },
  });
}

export function useCreateRfq() {
  const { desk } = useServices();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: NewRfq) => desk.createRfq(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.rfqs }),
  });
}

export function useCancelRfq() {
  const { desk } = useServices();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (rfqId: string) => desk.cancelRfq(rfqId),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.rfqs }),
  });
}

export function useSubmitQuote() {
  const { desk } = useServices();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ rfqId, quote }: { rfqId: string; quote: NewQuote }) => desk.submitQuote(rfqId, quote),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: queryKeys.rfqs }),
        qc.invalidateQueries({ queryKey: queryKeys.pendingActions }),
      ]);
    },
  });
}

export function useAcceptQuote() {
  const { desk } = useServices();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (quoteId: string) => desk.acceptQuote(quoteId),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: queryKeys.rfqs }),
        qc.invalidateQueries({ queryKey: queryKeys.trades }),
        qc.invalidateQueries({ queryKey: queryKeys.pendingActions }),
      ]);
    },
  });
}

export function useSetMakerMode() {
  const { auth } = useServices();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enabled: boolean) => auth.setMakerMode(enabled),
    onSuccess: async (me: Me) => {
      qc.setQueryData(queryKeys.me, me);
      await qc.invalidateQueries({ queryKey: queryKeys.rfqs });
    },
  });
}

/** Ledger changes answer with the actions to sign: they go to the auto-signer at once. */
function useLedgerMutation<T>(run: (input: T) => Promise<SignAction[]>) {
  const qc = useQueryClient();
  const { submit } = useSigner();
  return useMutation({
    mutationFn: run,
    onSuccess: async (actions) => {
      submit(actions);
      await qc.invalidateQueries({ queryKey: queryKeys.wallet });
    },
  });
}

export function useFaucet() {
  const { wallet } = useServices();
  return useLedgerMutation<void>(() => wallet.faucet());
}

export function useAcceptIncoming() {
  const { wallet } = useServices();
  return useLedgerMutation((contractId: string) => wallet.acceptIncoming(contractId));
}

export function useWithdraw() {
  const { wallet } = useServices();
  return useLedgerMutation((input: Withdrawal) => wallet.withdraw(input));
}

const UNLOCK = ["unlock"];

/**
 * Unlock signing. A mutation, so a failure toasts; `unlocking` is shared by every caller
 * (the session runs one passkey ceremony at a time). The result says whether it is still
 * the same wallet (false: the passkey opened another one and the session switched to it).
 */
export function useUnlock() {
  const session = useSession();
  const { mutate } = useMutation({ mutationKey: UNLOCK, mutationFn: () => session.unlock() });
  const unlocking = useIsMutating({ mutationKey: UNLOCK }) > 0;
  return { unlock: mutate, unlocking };
}

/**
 * For a control whose action needs a signature: `act` runs `run` straight away while signing
 * is unlocked, and otherwise unlocks first and runs it after — unless the unlock switched to
 * another wallet, whose user did not ask for it.
 */
export function useUnlockThen(run: () => void) {
  const { locked } = useSession();
  const { unlock, unlocking } = useUnlock();
  const act = (): void => {
    if (!locked) run();
    else
      unlock(undefined, {
        onSuccess: (sameWallet) => {
          if (sameWallet) run();
        },
      });
  };
  return { act, locked, unlocking };
}
