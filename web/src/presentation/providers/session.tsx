/**
 * Who is signed in, read through GET /me, and the party seed unlocked in this
 * tab. The session itself is the HttpOnly cookie when the gateway is the page's
 * own origin, or the token kept in this tab's sessionStorage when it is on
 * another origin (config.sessionTransport). The seed lives in memory only: a
 * reload keeps the session but locks signing until the user unlocks again.
 *
 * The session also keeps the auto-signer on the right identity: every sign-in,
 * an unlock that turned out to open another wallet, and an identity change the
 * server reports (an expired session, a sign-in in another tab) reset it before
 * anything else can be signed, and log-out locks it first thing.
 */
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
  /** The signed-in user; null unless status is "signed-in". */
  me: Me | null;
  /** The unlocked party seed of this tab. */
  seed: Uint8Array | null;
  /** Signed in with a passkey wallet whose key is not unlocked here. */
  locked: boolean;
  retry: () => void;
  createWallet: (displayName: string, onStep?: (step: CreateStep) => void) => Promise<void>;
  connectWallet: () => Promise<void>;
  /**
   * Unlock signing with a passkey (a fresh sign-in). Resolves true when it is still the same
   * wallet, false when the passkey opened another one and the session switched to it.
   * Concurrent calls share one passkey ceremony.
   */
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
  // A seed belongs to one user: another session (or none) sees no key at all.
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

  /** Make `wallet` the session; the signer switches synchronously (a new identity resets it). */
  const adopt = useCallback(
    (wallet: Wallet) => {
      if (wallet.me.id !== userId) {
        // Nothing an earlier identity read may linger or flash up: drop it, re-read what is shown.
        void qc.resetQueries({ predicate: (q) => q.queryKey[0] !== queryKeys.me[0] });
      }
      void signer.serve(wallet.me.id, wallet.seed);
      qc.setQueryData(queryKeys.me, wallet.me);
      setUnlocked({ userId: wallet.me.id, seed: wallet.seed });
    },
    [qc, signer, userId],
  );

  // Identity changes the server reports (an expired session, a sign-in in another tab) and every
  // lock or unlock: the signer serves exactly the current user with exactly its key.
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
    // Lock first (synchronously): nothing more is signed with this key, nothing it parked survives.
    void signer.serve(null, null);
    unlocked?.seed.fill(0);
    setUnlocked(null);
    try {
      await flows.logout();
    } finally {
      await qc.cancelQueries();
      // Keep the observed `me` entry (null) so the guards see the sign-out; drop everything else.
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
