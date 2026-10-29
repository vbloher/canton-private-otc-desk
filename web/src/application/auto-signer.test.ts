import { describe, expect, it } from "vitest";
import type { SignAction } from "@/domain/types";
import { createAutoSigner } from "./auto-signer";
import type { PartyKeys } from "./ports";

const enc = new TextEncoder();
const dec = new TextDecoder();

function action(id: string): SignAction {
  return {
    id,
    purpose: "allocate",
    description: `Allocate for ${id}`,
    hash: enc.encode(`hash-${id}`),
    createdAt: Date.parse("2026-10-21T12:00:00Z"),
    expiresAt: Date.parse("2026-10-21T12:10:00Z"),
  };
}

const keys: Pick<PartyKeys, "signer"> = {
  signer: (seed) => ({
    sign: async (message) => enc.encode(`${seed[0] ?? -1}:${dec.decode(message)}`),
  }),
};

const seed = (byte: number): Uint8Array => new Uint8Array(32).fill(byte);

function failure(status: number, message: string): Error {
  return Object.assign(new Error(message), { status });
}

function recorder(fail?: (actionId: string, call: number) => Error | undefined) {
  const calls: string[] = [];
  const execute = async (actionId: string, signature: Uint8Array): Promise<void> => {
    calls.push(`${actionId} ${dec.decode(signature)}`);
    const err = fail?.(actionId, calls.length);
    if (err !== undefined) throw err;
  };
  return { calls, execute };
}

describe("autoSigner", () => {
  it("signs and executes each action once", async () => {
    const { calls, execute } = recorder();
    const signer = createAutoSigner({ keys, execute });
    await signer.setSeed(seed(7));

    await signer.onActions([action("a1"), action("a2")]);
    await signer.onActions([action("a1"), action("a2")]);
    await Promise.all([signer.onActions([action("a3")]), signer.onActions([action("a3")])]);

    expect(calls).toEqual(["a1 7:hash-a1", "a2 7:hash-a2", "a3 7:hash-a3"]);
  });

  it("parks actions while locked, then drains them on setSeed", async () => {
    const { calls, execute } = recorder();
    const signer = createAutoSigner({ keys, execute });

    await signer.onActions([action("a1"), action("a2")]);
    expect(calls).toEqual([]);

    await signer.setSeed(seed(3));
    expect(calls).toEqual(["a1 3:hash-a1", "a2 3:hash-a2"]);

    await signer.onActions([action("a1"), action("a2")]);
    expect(calls).toHaveLength(2);
  });

  it("parks again after locking", async () => {
    const { calls, execute } = recorder();
    const signer = createAutoSigner({ keys, execute });
    await signer.setSeed(seed(1));
    await signer.setSeed(null);

    await signer.onActions([action("a1")]);
    expect(calls).toEqual([]);

    await signer.setSeed(seed(2));
    expect(calls).toEqual(["a1 2:hash-a1"]);
  });

  it("treats 409 as done", async () => {
    const errors: string[] = [];
    const { calls, execute } = recorder(() => failure(409, "action is not pending"));
    const signer = createAutoSigner({ keys, execute, onError: (a) => errors.push(a.id) });
    await signer.setSeed(seed(1));

    await signer.onActions([action("a1")]);
    await signer.onActions([action("a1")]);

    expect(calls).toHaveLength(1);
    expect(errors).toEqual([]);
  });

  it("treats 404 as done", async () => {
    const errors: string[] = [];
    const { calls, execute } = recorder(() => failure(404, "unknown action"));
    const signer = createAutoSigner({ keys, execute, onError: (a) => errors.push(a.id) });
    await signer.setSeed(seed(1));

    await signer.onActions([action("a1")]);
    await signer.onActions([action("a1")]);

    expect(calls).toHaveLength(1);
    expect(errors).toEqual([]);
  });

  it("reports any other failure and retries on the next delivery", async () => {
    const errors: string[] = [];
    const { calls, execute } = recorder((_id, call) => (call === 1 ? failure(502, "ledger hiccup") : undefined));
    const signer = createAutoSigner({
      keys,
      execute,
      onError: (a, e) => errors.push(`${a.id}: ${(e as Error).message}`),
    });
    await signer.setSeed(seed(1));

    await signer.onActions([action("a1")]);
    expect(errors).toEqual(["a1: ledger hiccup"]);

    await signer.onActions([action("a1")]);
    expect(calls).toEqual(["a1 1:hash-a1", "a1 1:hash-a1"]);
    expect(errors).toHaveLength(1);
  });

  it("never signs an action that completed elsewhere", async () => {
    const { calls, execute } = recorder();
    const signer = createAutoSigner({ keys, execute });
    await signer.onActions([action("a1"), action("a3")]);

    signer.markDone("a1");
    signer.markDone("a2");
    await signer.setSeed(seed(1));
    await signer.onActions([action("a2")]);

    expect(calls).toEqual(["a3 1:hash-a3"]);
  });

  it("tells the caller about every executed action", async () => {
    const executed: string[] = [];
    const { execute } = recorder();
    const signer = createAutoSigner({ keys, execute, onExecuted: (a) => executed.push(a.id) });
    await signer.setSeed(seed(1));

    await signer.onActions([action("a1"), action("a2")]);

    expect(executed).toEqual(["a1", "a2"]);
  });

  it("tells the caller when the desk discarded an action (404 or 409)", async () => {
    const discarded: string[] = [];
    const { execute } = recorder((id) => (id === "gone" ? failure(404, "unknown") : failure(409, "not pending")));
    const signer = createAutoSigner({ keys, execute, onDiscarded: (a) => discarded.push(a.id) });
    await signer.setSeed(seed(1));

    await signer.onActions([action("gone"), action("expired")]);

    expect(discarded).toEqual(["gone", "expired"]);
  });

  it("serves one identity at a time: the next never drains the last one's actions, a returning one is signed for again", async () => {
    const { calls, execute } = recorder();
    const signer = createAutoSigner({ keys, execute });
    await signer.serve("A", null);
    await signer.onActions([action("a1")]);

    await signer.serve(null, null);
    await signer.serve("B", seed(2));
    expect(calls).toEqual([]);

    await signer.serve("A", seed(1));
    await signer.onActions([action("a1")]);
    expect(calls).toEqual(["a1 1:hash-a1"]);
  });

  it("keeps an identity's parked actions while it only locks and unlocks", async () => {
    const { calls, execute } = recorder();
    const signer = createAutoSigner({ keys, execute });
    await signer.serve("A", seed(1));
    await signer.serve("A", null);
    await signer.onActions([action("a1")]);

    await signer.serve("A", seed(1));

    expect(calls).toEqual(["a1 1:hash-a1"]);
  });

  it("never drains the previous identity's parked actions into a new identity", async () => {
    const { calls, execute } = recorder();
    const signer = createAutoSigner({ keys, execute });
    await signer.onActions([action("a1")]);

    signer.reset();
    await signer.setSeed(seed(2));

    expect(calls).toEqual([]);
  });

  it("signs a re-connected identity's actions again", async () => {
    const { calls, execute } = recorder((_id, call) => (call === 1 ? failure(404, "not your action") : undefined));
    const signer = createAutoSigner({ keys, execute });
    await signer.setSeed(seed(2));
    await signer.onActions([action("a1")]);

    signer.reset();
    await signer.setSeed(seed(1));
    await signer.onActions([action("a1")]);

    expect(calls).toEqual(["a1 2:hash-a1", "a1 1:hash-a1"]);
  });

  it("drops what was in flight when it reset: no outcome reported, the rest of the batch not kept", async () => {
    const reported: string[] = [];
    const calls: string[] = [];
    let release: (() => void) | undefined;
    const execute = (actionId: string): Promise<void> => {
      calls.push(actionId);
      return actionId === "a1"
        ? new Promise((_resolve, reject) => {
            release = () => reject(failure(502, "ledger hiccup"));
          })
        : Promise.resolve();
    };
    const signer = createAutoSigner({
      keys,
      execute,
      onError: (a) => reported.push(`error ${a.id}`),
      onExecuted: (a) => reported.push(`executed ${a.id}`),
    });
    await signer.setSeed(seed(1));

    const batch = signer.onActions([action("a1"), action("a2")]);
    await Promise.resolve();
    signer.reset();
    release?.();
    await batch;
    await signer.setSeed(seed(2));

    expect(calls).toEqual(["a1"]);
    expect(reported).toEqual([]);
  });
});
