/**
 * The one auto-signer of this tab, above the session: the session resets and
 * re-keys it synchronously whenever the identity or the key changes, so
 * nothing is ever signed with one identity's key for another.
 */
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { createContext, use, useCallback, useEffect, useEffectEvent, useMemo, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { createAutoSigner, type AutoSigner } from "@/application/auto-signer";
import { errorMessage } from "@/application/errors";
import { LATE_REFRESH_MS } from "@/application/event-invalidation";
import { queryKeys } from "@/application/query-keys";
import type { SignAction } from "@/domain/types";
import { useServices } from "./services";

type DroppedListener = (actionId: string) => void;

interface AutoSignerState {
  signer: AutoSigner;
  /** Listen for actions this tab will not get executed after all: failed, gone or expired. */
  subscribeDropped: (listener: DroppedListener) => () => void;
}

const AutoSignerContext = createContext<AutoSignerState | null>(null);

/** Holdings change when the ledger commits, a moment after the execute returned. */
function refreshWallet(qc: QueryClient): void {
  void qc.invalidateQueries({ queryKey: queryKeys.wallet });
  setTimeout(() => void qc.invalidateQueries({ queryKey: queryKeys.wallet }), LATE_REFRESH_MS);
}

export function AutoSignerProvider({ children }: { children: ReactNode }) {
  const services = useServices();
  const qc = useQueryClient();
  const [listeners] = useState(() => new Set<DroppedListener>());
  const [signer] = useState(() => {
    const dropped = (action: SignAction): void => {
      for (const listener of listeners) listener(action.id);
    };
    return createAutoSigner({
      keys: services.keys,
      execute: (id, signature) => services.wallet.executeAction(id, signature),
      onExecuted: () => {
        void qc.invalidateQueries({ queryKey: queryKeys.pendingActions });
        refreshWallet(qc);
      },
      onError: (action, e) => {
        toast.error(`Could not sign: ${action.description}`, { id: `sign:${action.id}`, description: errorMessage(e) });
        dropped(action);
      },
      onDiscarded: dropped,
    });
  });

  const subscribeDropped = useCallback(
    (listener: DroppedListener) => {
      listeners.add(listener);
      return () => void listeners.delete(listener);
    },
    [listeners],
  );

  const value = useMemo<AutoSignerState>(() => ({ signer, subscribeDropped }), [signer, subscribeDropped]);
  return <AutoSignerContext value={value}>{children}</AutoSignerContext>;
}

export function useAutoSigner(): AutoSignerState {
  const state = use(AutoSignerContext);
  if (state === null) throw new Error("useAutoSigner must be used inside <AutoSignerProvider>");
  return state;
}

/** Run `handler` with the id of every action this tab will not get executed after all. */
export function useSignDropped(handler: (actionId: string) => void): void {
  const { subscribeDropped } = useAutoSigner();
  const onDropped = useEffectEvent(handler);
  useEffect(() => subscribeDropped((actionId) => onDropped(actionId)), [subscribeDropped]);
}
