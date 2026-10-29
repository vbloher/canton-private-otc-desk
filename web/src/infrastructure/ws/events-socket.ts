import type { DeskEvent, DeskEventType, EventHandlers, EventsSource } from "@/application/ports";

export interface SocketLike {
  onopen: ((ev: Event) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  onerror: ((ev: Event) => void) | null;
  close(): void;
}

const FIRST_DELAY_MS = 500;
const MAX_DELAY_MS = 8_000;

export function reconnectDelay(attempt: number): number {
  return Math.min(FIRST_DELAY_MS * 2 ** (attempt - 1), MAX_DELAY_MS);
}

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
  url: () => string;
  open?: (url: string) => SocketLike;
}

export function deskEventsUrl(location: Pick<Location, "protocol" | "host">): string {
  return `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/api/v1/ws`;
}

export function createEventsSource(options: EventsSourceOptions): EventsSource {
  const open = options.open ?? ((url: string): SocketLike => new WebSocket(url));
  return {
    connect(handlers: EventHandlers): () => void {
      let stopped = false;
      let attempt = 0;
      let socket: SocketLike | undefined;
      let timer: ReturnType<typeof setTimeout> | undefined;

      const dial = (): void => {
        if (stopped) return;
        handlers.onStatus("connecting");
        const s = open(options.url());
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
        s.onerror = () => undefined;
        s.onclose = () => {
          if (stopped || socket !== s) return;
          socket = undefined;
          handlers.onStatus("offline");
          attempt += 1;
          timer = setTimeout(dial, reconnectDelay(attempt));
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
