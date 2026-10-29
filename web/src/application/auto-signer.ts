import type { SignAction } from "@/domain/types";
import { isApiFailure } from "./errors";
import type { PartyKeys, Signer } from "./ports";

export interface AutoSignerDeps {
  keys: Pick<PartyKeys, "signer">;
  execute(actionId: string, signature: Uint8Array): Promise<void>;
  onError?(action: SignAction, error: unknown): void;
  onDiscarded?(action: SignAction): void;
  onExecuted?(action: SignAction): void;
}

export interface AutoSigner {
  onActions(actions: readonly SignAction[]): Promise<void>;
  serve(identity: string | null, seed: Uint8Array | null): Promise<void>;
  setSeed(seed: Uint8Array | null): Promise<void>;
  markDone(actionId: string): void;
  reset(): void;
}

const HANDLED_CAP = 1000;

const settledElsewhere = (e: unknown): boolean => isApiFailure(e) && (e.status === 404 || e.status === 409);

export function createAutoSigner(deps: AutoSignerDeps): AutoSigner {
  let signer: Signer | null = null;
  let identity: string | null = null;
  let generation = 0;
  const handled = new Set<string>();
  const parked = new Map<string, SignAction>();

  function remember(id: string): void {
    handled.add(id);
    for (const old of handled) {
      if (handled.size <= HANDLED_CAP) break;
      handled.delete(old);
    }
  }

  async function signOne(action: SignAction): Promise<void> {
    if (handled.has(action.id)) return;
    const current = signer;
    if (current === null) {
      parked.set(action.id, action);
      return;
    }
    const started = generation;
    parked.delete(action.id);
    remember(action.id);
    try {
      await deps.execute(action.id, await current.sign(action.hash));
      if (started === generation) deps.onExecuted?.(action);
    } catch (e) {
      if (started !== generation) return;
      if (settledElsewhere(e)) {
        deps.onDiscarded?.(action);
        return;
      }
      handled.delete(action.id);
      deps.onError?.(action, e);
    }
  }

  async function signAll(actions: readonly SignAction[]): Promise<void> {
    const started = generation;
    for (const action of actions) {
      if (started !== generation) return;
      await signOne(action);
    }
  }

  async function setSeed(seed: Uint8Array | null): Promise<void> {
    signer = seed === null ? null : deps.keys.signer(seed);
    if (signer === null) return;
    const queued = [...parked.values()];
    parked.clear();
    await signAll(queued);
  }

  function reset(): void {
    generation += 1;
    signer = null;
    identity = null;
    handled.clear();
    parked.clear();
  }

  return {
    onActions: signAll,
    async serve(next, seed) {
      if (next !== identity) {
        reset();
        identity = next;
      }
      await setSeed(seed);
    },
    setSeed,
    markDone(actionId) {
      parked.delete(actionId);
      remember(actionId);
    },
    reset,
  };
}
