import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DeskEvent, SocketStatus } from "@/application/ports";
import { createEventsSource, parseEvent, type SocketLike } from "./events-socket";

class FakeSocket implements SocketLike {
  static all: FakeSocket[] = [];
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  closed = false;
  readonly url: string;

  constructor(url: string) {
    this.url = url;
    FakeSocket.all.push(this);
  }

  open(): void {
    this.onopen?.(new Event("open"));
  }

  receive(data: string): void {
    this.onmessage?.(new MessageEvent("message", { data }));
  }

  /** The server or the network went away (a browser closes the socket asynchronously, a test at once). */
  drop(): void {
    this.onclose?.(new Event("close") as CloseEvent);
  }

  close(): void {
    this.closed = true;
    this.drop();
  }
}

const latest = (): FakeSocket => {
  const s = FakeSocket.all.at(-1);
  if (s === undefined) throw new Error("no socket opened");
  return s;
};

function connect() {
  const events: DeskEvent[] = [];
  const statuses: SocketStatus[] = [];
  let opens = 0;
  const source = createEventsSource({ url: () => "ws://desk.test/api/v1/ws", open: (url) => new FakeSocket(url) });
  const stop = source.connect({
    onEvent: (e) => events.push(e),
    onStatus: (s) => statuses.push(s),
    onOpen: () => void opens++,
  });
  return { events, statuses, stop, opens: () => opens };
}

describe("parseEvent", () => {
  it("reads the desk's {type, data} frames", () => {
    expect(parseEvent('{"type":"rfq.created","data":{"rfqId":"r1"}}')).toEqual({
      type: "rfq.created",
      data: { rfqId: "r1" },
    });
    expect(parseEvent('{"type":"wallet.updated","data":{}}')).toEqual({ type: "wallet.updated", data: {} });
    expect(parseEvent('{"type":"trade.updated","data":{"tradeId":"t1","rfqId":"r1","status":"settled"}}')?.type).toBe(
      "trade.updated",
    );
  });

  it("drops anything else", () => {
    expect(parseEvent("not json")).toBeNull();
    expect(parseEvent('{"type":"desk.restarted","data":{}}')).toBeNull();
    expect(parseEvent('{"type":"rfq.created"}')).toBeNull();
    expect(parseEvent('{"type":"rfq.created","data":null}')).toBeNull();
    expect(parseEvent('{"type":"trade.updated","data":{"tradeId":"t1"}}')).toBeNull();
    expect(parseEvent("[1,2]")).toBeNull();
  });
});

describe("createEventsSource", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    FakeSocket.all = [];
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("connects to the url, reports live and delivers events", () => {
    const c = connect();
    expect(latest().url).toBe("ws://desk.test/api/v1/ws");
    expect(c.statuses).toEqual(["connecting"]);

    latest().open();
    latest().receive('{"type":"sign.requested","data":{"actionId":"a1"}}');
    latest().receive("garbage");

    expect(c.statuses).toEqual(["connecting", "live"]);
    expect(c.opens()).toBe(1);
    expect(c.events).toEqual([{ type: "sign.requested", data: { actionId: "a1" } }]);
  });

  it("reconnects after 0.5 s, doubling up to 8 s", () => {
    const c = connect();
    const delays: number[] = [];
    for (let attempt = 0; attempt < 6; attempt++) {
      const before = FakeSocket.all.length;
      latest().drop();
      let waited = 0;
      while (FakeSocket.all.length === before) {
        vi.advanceTimersByTime(100);
        waited += 100;
        if (waited > 60_000) throw new Error("never reconnected");
      }
      delays.push(waited);
    }

    expect(delays).toEqual([500, 1000, 2000, 4000, 8000, 8000]);
    expect(c.statuses.filter((s) => s === "offline")).toHaveLength(6);
  });

  it("starts the backoff over once a connection opened", () => {
    connect();
    latest().drop();
    vi.advanceTimersByTime(500);
    latest().drop();
    vi.advanceTimersByTime(1000);
    latest().open();

    const before = FakeSocket.all.length;
    latest().drop();
    vi.advanceTimersByTime(499);
    expect(FakeSocket.all).toHaveLength(before);
    vi.advanceTimersByTime(1);
    expect(FakeSocket.all).toHaveLength(before + 1);
  });

  it("stops for good when disconnected", () => {
    const c = connect();
    latest().open();
    const socket = latest();

    c.stop();
    vi.advanceTimersByTime(60_000);

    expect(socket.closed).toBe(true);
    expect(FakeSocket.all).toHaveLength(1);
    expect(c.statuses).toEqual(["connecting", "live"]);
  });
});
