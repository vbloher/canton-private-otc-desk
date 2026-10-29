import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createContext, use, useCallback, useEffect, useMemo, type ReactNode } from "react";
import { queryKeys } from "@/application/query-keys";
import type { SignAction } from "@/domain/types";
import { useAutoSigner } from "./auto-signer";
import { useDeskEvent } from "./events";
import { useServices } from "./services";
import { useSession } from "./session";

const POLL_MS = 5_000;

interface SignerState {
  pending: SignAction[];
  submit: (actions: readonly SignAction[]) => void;
}

const SignerContext = createContext<SignerState | null>(null);

export function SignerProvider({ children }: { children: ReactNode }) {
  const services = useServices();
  const qc = useQueryClient();
  const session = useSession();
  const { signer } = useAutoSigner();

  const pending = useQuery({
    queryKey: queryKeys.pendingActions,
    queryFn: () => services.wallet.pendingActions(),
    enabled: session.status === "signed-in",
    refetchInterval: POLL_MS,
    refetchIntervalInBackground: true,
    meta: { quiet: true },
  });

  const { data: actions, dataUpdatedAt } = pending;
  useEffect(() => {
    if (actions !== undefined && dataUpdatedAt > 0) void signer.onActions(actions);
  }, [signer, actions, dataUpdatedAt]);

  useDeskEvent((event) => {
    if (event.type === "sign.completed") signer.markDone(event.data.actionId);
  });

  const submit = useCallback(
    (queued: readonly SignAction[]) => {
      void signer.onActions(queued);
      void qc.invalidateQueries({ queryKey: queryKeys.pendingActions });
    },
    [signer, qc],
  );

  const value = useMemo<SignerState>(
    () => ({ pending: session.status === "signed-in" ? (actions ?? []) : [], submit }),
    [session.status, actions, submit],
  );
  return <SignerContext value={value}>{children}</SignerContext>;
}

export function useSigner(): SignerState {
  const state = use(SignerContext);
  if (state === null) throw new Error("useSigner must be used inside <SignerProvider>");
  return state;
}
