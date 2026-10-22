/**
 * The auto-signer: once the wallet is unlocked, every pending sign action is
 * signed with the party key and executed, without asking (the spec signs
 * blindly, as the reference desk did). It is fed by `sign.requested` pushes,
 * the 5 s poll of /tx/pending and the actions a mutation returns, so the same
 * action arrives several times: each id is signed at most once. Actions that
 * arrive while the key is locked are parked and drained on unlock.
 *
 * Everything it holds belongs to one identity: `serve(identity, seed)` keys it
 * to the signed-in user, and a different user (or none) first resets it — the
 * key, the parked actions and the ids seen are forgotten, and whatever was in
 * flight at that moment reports nothing to the identity that comes next.
 */
import type { SignAction } from "@/domain/types";
import { isApiFailure } from "./errors";
import type { PartyKeys, Signer } from "./ports";

export interface AutoSignerDeps {
  keys: Pick<PartyKeys, "signer">;
  execute(actionId: string, signature: Uint8Array): Promise<void>;
  /** A signature or execute failed for a reason worth telling the user; it is retried later. */
  onError?(action: SignAction, error: unknown): void;
  /** The desk dropped the action first: it is gone (404) or no longer pending (409). */
  onDiscarded?(action: SignAction): void;
  onExecuted?(action: SignAction): void;
}

export interface AutoSigner {
  /** Sign and execute every action not handled yet (parks them while locked). */
  onActions(actions: readonly SignAction[]): Promise<void>;
  /**
   * Sign for `identity` with `seed` (null: locked). A different identity than the one served
   * resets first; the same identity only locks or unlocks, and unlocking drains what it parked.
   */
  serve(identity: string | null, seed: Uint8Array | null): Promise<void>;
  /** A seed unlocks signing and drains the parked actions; null locks it again (same identity). */
  setSeed(seed: Uint8Array | null): Promise<void>;
  /** The action completed elsewhere (another tab): never sign it here. */
  markDone(actionId: string): void;
  /** Another identity takes over, or none: drop the key and forget every action seen so far. */
  reset(): void;
}

/** Bounds the dedup memory of a long-lived tab; old ids can no longer be offered. */
const HANDLED_CAP = 1000;

/**
 * 404: the action is gone (not ours, or already cleaned up). 409: it is no
 * longer pending (executed by another tab, failed, or expired). Either way
 * there is nothing left to sign.
 */
const settledElsewhere = (e: unknown): boolean => isApiFailure(e) && (e.status === 404 || e.status === 409);

export function createAutoSigner(deps: AutoSignerDeps): AutoSigner {
  let signer: Signer | null = null;
  let identity: string | null = null;
  /** Bumped by reset(): work started under an older generation belongs to an identity that left. */
  let generation = 0;
  const handled = new Set<string>();
  const parked = new Map<string, SignAction>();

  function remember(id: string): void {
    handled.add(id);
    for (const old of handled) {
      if (handled.size <= HANDLED_CAP) break;
      handled.delete(old); // insertion order: the oldest go first
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
    remember(action.id); // before the first await: a concurrent delivery must not sign twice
    try {
      await deps.execute(action.id, await current.sign(action.hash));
      if (started === generation) deps.onExecuted?.(action);
    } catch (e) {
      if (started !== generation) return;
      if (settledElsewhere(e)) {
        deps.onDiscarded?.(action);
        return;
      }
      handled.delete(action.id); // the next poll offers it again
      deps.onError?.(action, e);
    }
  }

  async function signAll(actions: readonly SignAction[]): Promise<void> {
    const started = generation;
    for (const action of actions) {
      if (started !== generation) return; // reset midway: the rest was the previous identity's
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
