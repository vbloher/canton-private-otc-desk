import { useQueryClient } from "@tanstack/react-query";
import { createContext, use, useCallback, useEffect, useEffectEvent, useMemo, useState, type ReactNode } from "react";
import { invalidationsFor, LATE_REFRESH_MS, lateInvalidationsFor } from "@/application/event-invalidation";
import type { DeskEvent, SocketStatus } from "@/application/ports";
import { useServices } from "./services";
import { useSession } from "./session";

type Listener = (event: DeskEvent) => void;

interface Events {
  status: SocketStatus;
  subscribe: (listener: Listener) => () => void;
}

const EventsContext = createContext<Events | null>(null);

export function EventsProvider({ children }: { children: ReactNode }) {
  const services = useServices();
  const qc = useQueryClient();
  const userId = useSession().me?.id ?? null;
  const signedIn = userId !== null;
  const [status, setStatus] = useState<SocketStatus>("offline");
  const [listeners] = useState(() => new Set<Listener>());

  useEffect(() => {
    if (userId === null) return;
    const timers = new Set<ReturnType<typeof setTimeout>>();
    const disconnect = services.events.connect({
      onStatus: setStatus,
      onOpen: () => void qc.invalidateQueries(),
      onEvent: (event) => {
        for (const queryKey of invalidationsFor(event.type)) void qc.invalidateQueries({ queryKey });
        const later = lateInvalidationsFor(event.type);
        if (later.length > 0) {
          const timer = setTimeout(() => {
            timers.delete(timer);
            for (const queryKey of later) void qc.invalidateQueries({ queryKey });
          }, LATE_REFRESH_MS);
          timers.add(timer);
        }
        for (const listener of listeners) listener(event);
      },
    });
    return () => {
      disconnect();
      for (const timer of timers) clearTimeout(timer);
    };
  }, [userId, services, qc, listeners]);

  const subscribe = useCallback(
    (listener: Listener) => {
      listeners.add(listener);
      return () => void listeners.delete(listener);
    },
    [listeners],
  );

  const value = useMemo<Events>(
    () => ({ status: signedIn ? status : "offline", subscribe }),
    [signedIn, status, subscribe],
  );
  return <EventsContext value={value}>{children}</EventsContext>;
}

function useEvents(): Events {
  const events = use(EventsContext);
  if (events === null) throw new Error("events hooks must be used inside <EventsProvider>");
  return events;
}

export function useSocketStatus(): SocketStatus {
  return useEvents().status;
}

export function useDeskEvent(handler: (event: DeskEvent) => void): void {
  const { subscribe } = useEvents();
  const onEvent = useEffectEvent(handler);
  useEffect(() => subscribe((event) => onEvent(event)), [subscribe]);
}
