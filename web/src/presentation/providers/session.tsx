import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createContext, use, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { createAuthFlows, type CreateStep, type Wallet } from "@/application/auth";
import { queryKeys } from "@/application/query-keys";
import type { Me } from "@/domain/types";
import { useServices } from "./services";
import { useAutoSigner } from "./auto-signer";

export type SessionStatus = "loading" | "error" | "signed-out" | "signed-in";

export interface Session {
  status: SessionStatus;
  me: Me | null;
  seed: Uint8Array | null;
  locked: boolean;
  retry: () => void;
  createWallet: (displayName: string, onStep?: (step: CreateStep) => void) => Promise<void>;
  connectWallet: () => Promise<void>;
  unlock: () => Promise<boolean>;
  logout: () => Promise<void>;
}

const SessionContext = createContext<Session | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const services = useServices();
  const qc = useQueryClient();
  const { signer } = useAutoSigner();
  const flows = useMemo(() => createAuthFlows(services), [services]);
  const meQuery = useQuery({
    queryKey: queryKeys.me,
    queryFn: () => services.auth.me(),
    retry: false,
    staleTime: 30_000,
  });
  const [unlocked, setUnlocked] = useState<{ userId: string; seed: Uint8Array } | null>(null);
  const unlocking = useRef<Promise<boolean> | null>(null);

  const me = meQuery.data ?? null;
  const seed = unlocked !== null && me !== null && unlocked.userId === me.id ? unlocked.seed : null;
  const status: SessionStatus =
    meQuery.data !== undefined
      ? me === null
        ? "signed-out"
        : "signed-in"
      : meQuery.isError
        ? "error"
        : "loading";

  const userId = me?.id ?? null;

  const adopt = useCallback(
    (wallet: Wallet) => {
      if (wallet.me.id !== userId) {
        void qc.resetQueries({ predicate: (q) => q.queryKey[0] !== queryKeys.me[0] });
      }
      void signer.serve(wallet.me.id, wallet.seed);
      qc.setQueryData(queryKeys.me, wallet.me);
      setUnlocked({ userId: wallet.me.id, seed: wallet.seed });
    },
    [qc, signer, userId],
  );

  useEffect(() => {
    void signer.serve(userId, seed);
  }, [signer, userId, seed]);

  const createWallet = useCallback(
    async (displayName: string, onStep?: (step: CreateStep) => void) => {
      adopt(await flows.createWallet(displayName, onStep));
    },
    [flows, adopt],
  );

  const connectWallet = useCallback(async () => {
    adopt(await flows.connectWallet());
  }, [flows, adopt]);

  const unlock = useCallback((): Promise<boolean> => {
    if (me === null) return Promise.reject(new Error("Connect your wallet first."));
    unlocking.current ??= flows
      .unlock(me)
      .then((wallet) => {
        const same = wallet.me.id === me.id;
        if (!same) {
          toast(`Signed in as ${wallet.me.displayName}`, {
            description: "That passkey belongs to another wallet: the desk switched to it.",
          });
        }
        adopt(wallet);
        return same;
      })
      .finally(() => {
        unlocking.current = null;
      });
    return unlocking.current;
  }, [flows, me, adopt]);

  const logout = useCallback(async () => {
    void signer.serve(null, null);
    unlocked?.seed.fill(0);
    setUnlocked(null);
    try {
      await flows.logout();
    } finally {
      await qc.cancelQueries();
      qc.setQueryData(queryKeys.me, null);
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== queryKeys.me[0] });
    }
  }, [flows, qc, signer, unlocked]);

  const { refetch } = meQuery;
  const retry = useCallback(() => void refetch(), [refetch]);

  const value = useMemo<Session>(
    () => ({
      status,
      me,
      seed,
      locked: me !== null && me.kind === "passkey" && seed === null,
      retry,
      createWallet,
      connectWallet,
      unlock,
      logout,
    }),
    [status, me, seed, retry, createWallet, connectWallet, unlock, logout],
  );
  return <SessionContext value={value}>{children}</SessionContext>;
}

export function useSession(): Session {
  const session = use(SessionContext);
  if (session === null) throw new Error("useSession must be used inside <SessionProvider>");
  return session;
}
