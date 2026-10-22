/**
 * One shared clock for every countdown: a single interval while anything
 * listens, read through useSyncExternalStore so renders stay pure.
 */
import { useSyncExternalStore } from "react";

const TICK_MS = 1_000;
let now = Date.now();
let timer: ReturnType<typeof setInterval> | undefined;
const listeners = new Set<() => void>();

function tick(): void {
  now = Date.now();
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  if (timer === undefined) {
    now = Date.now(); // a stale reading from before the clock slept
    timer = setInterval(tick, TICK_MS);
  }
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0 && timer !== undefined) {
      clearInterval(timer);
      timer = undefined;
    }
  };
}

const read = (): number => now;

/** Epoch ms, re-rendering about once a second. */
export function useNow(): number {
  return useSyncExternalStore(subscribe, read, read);
}
