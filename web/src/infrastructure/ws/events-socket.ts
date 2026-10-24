/**
 * The desk's event stream: one WebSocket on /api/v1/ws, `{"type", "data"}`
 * frames, reconnecting with a 0.5 s backoff that doubles up to 8 s and starts
 * over after a good connection. The upgrade is authenticated by the session
 * cookie on the same origin, or by the session token offered as the
 * subprotocol `otc-desk.bearer.<token>` (next to `otc-desk.v1`, which the
 * gateway always echoes) when the gateway is on another origin.
 */
import type { DeskEvent, DeskEventType, EventHandlers, EventsSource } from "@/application/ports";

/** The part of a browser WebSocket this client uses. */
export interface SocketLike {
  onopen: ((ev: Event) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  onerror: ((ev: Event) => void) | null;
  close(): void;
}

export const PROTOCOL = "otc-desk.v1";
const BEARER_PREFIX = "otc-desk.bearer.";
/** Characters an HTTP token, and so a subprotocol name, may hold (RFC 7230 tchar). */
const TCHARS = /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/;

/** The subprotocols to offer: always the desk's, plus the session token when one is known. */
export function eventProtocols(token: string | undefined): string[] {
  return token !== undefined && TCHARS.test(token) ? [PROTOCOL, `${BEARER_PREFIX}${token}`] : [PROTOCOL];
}

const FIRST_DELAY_MS = 500;
const MAX_DELAY_MS = 8_000;

/** Attempt 1 waits 0.5 s, then 1, 2, 4 and 8 s from there on. */
export function reconnectDelay(attempt: number): number {
  return Math.min(FIRST_DELAY_MS * 2 ** (attempt - 1), MAX_DELAY_MS);
}

/** Fields each event's data must carry, as strings. */
const FIELDS: Record<DeskEventType, readonly string[]> = {
  "rfq.created": ["rfqId"],
  "rfq.updated": ["rfqId"],
  "quote.updated": ["quoteId", "rfqId", "status"],
  "trade.updated": ["tradeId", "rfqId", "status"],
  "sign.requested": ["actionId"],
  "sign.completed": ["actionId"],
  "wallet.updated": [],
};

const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);

/** A frame from the gateway, or null for anything that is not a desk event. */
export function parseEvent(raw: string): DeskEvent | null {
  let frame: unknown;
  try {
    frame = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!isRecord(frame) || typeof frame.type !== "string" || !isRecord(frame.data)) return null;
  if (!Object.hasOwn(FIELDS, frame.type)) return null;
  const data = frame.data;
  const fields = FIELDS[frame.type as DeskEventType];
  if (!fields.every((f) => typeof data[f] === "string")) return null;
  return { type: frame.type, data } as DeskEvent;
}

export interface EventsSourceOptions {
  /** Read at every dial (config.eventsUrl). */
  url: () => string;
  /** Read at every dial, so a reconnect offers the current session token (eventProtocols). */
  protocols?: () => string[];
  open?: (url: string, protocols: string[]) => SocketLike;
}

export function createEventsSource(options: EventsSourceOptions): EventsSource {
  const open = options.open ?? ((url: string, protocols: string[]): SocketLike => new WebSocket(url, protocols));
  const protocols = options.protocols ?? (() => [PROTOCOL]);
  return {
    connect(handlers: EventHandlers): () => void {
      let stopped = false;
      let attempt = 0;
      let socket: SocketLike | undefined;
      let timer: ReturnType<typeof setTimeout> | undefined;

      const retry = (): void => {
        handlers.onStatus("offline");
        attempt += 1;
        timer = setTimeout(dial, reconnectDelay(attempt));
      };

      const dial = (): void => {
        if (stopped) return;
        handlers.onStatus("connecting");
        let s: SocketLike;
        try {
          s = open(options.url(), protocols());
        } catch {
          retry(); // e.g. a URL or subprotocol the browser refuses: wait, then try again
          return;
        }
        socket = s;
        s.onopen = () => {
          if (stopped || socket !== s) return;
          attempt = 0;
          handlers.onStatus("live");
          handlers.onOpen?.();
        };
        s.onmessage = (msg) => {
          if (stopped || socket !== s || typeof msg.data !== "string") return;
          const event = parseEvent(msg.data);
          if (event !== null) handlers.onEvent(event);
        };
        s.onerror = () => undefined; // a close always follows
        s.onclose = () => {
          if (stopped || socket !== s) return;
          socket = undefined;
          retry();
        };
      };

      dial();
      return () => {
        stopped = true;
        if (timer !== undefined) clearTimeout(timer);
        socket?.close();
        socket = undefined;
      };
    },
  };
}
