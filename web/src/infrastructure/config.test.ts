import { describe, expect, it } from "vitest";
import {
  apiBaseUrl,
  apiOrigin,
  checkPagesApiOrigin,
  deskMissing,
  eventsUrl,
  framingRefused,
  normalizeBase,
  routerBasename,
  sessionTransport,
} from "./config";

describe("sessionTransport", () => {
  it("keeps the HttpOnly cookie alone on the page's own origin", () => {
    expect(sessionTransport("")).toBe("cookie");
  });

  it("carries the token as a Bearer when the gateway is on another origin", () => {
    expect(sessionTransport("https://desk.example.com")).toBe("bearer");
  });
});

describe("framingRefused", () => {
  const top = {};
  const self = {};

  it("refuses a Pages build shown inside another page", () => {
    expect(framingRefused("pages", { top, self })).toBe(true);
  });

  it("lets a Pages build run at the top level", () => {
    expect(framingRefused("pages", { top: self, self })).toBe(false);
  });

  it("leaves other builds alone: nginx sends frame-ancestors 'none' for them", () => {
    expect(framingRefused(undefined, { top, self })).toBe(false);
  });

  it("refuses when the top window cannot even be inspected", () => {
    const hostile = {
      self,
      get top(): unknown {
        throw new DOMException("blocked", "SecurityError");
      },
    };
    expect(framingRefused("pages", hostile)).toBe(true);
  });
});

describe("checkPagesApiOrigin", () => {
  it("accepts an https gateway and loopback http for a Pages build", () => {
    expect(() => checkPagesApiOrigin("pages", "https://desk.example.com")).not.toThrow();
    expect(() => checkPagesApiOrigin("pages", "http://localhost:8080")).not.toThrow();
    expect(() => checkPagesApiOrigin("pages", "http://127.0.0.1:8080")).not.toThrow();
    expect(() => checkPagesApiOrigin("pages", "http://[::1]:8080")).not.toThrow();
    expect(() => checkPagesApiOrigin("pages", "")).not.toThrow();
  });

  it("refuses a plain-http gateway elsewhere: the https page could not reach it", () => {
    expect(() => checkPagesApiOrigin("pages", "http://desk.example.com")).toThrow(/https/);
  });

  it("leaves other deployments to choose", () => {
    expect(() => checkPagesApiOrigin(undefined, "http://desk.intranet")).not.toThrow();
  });
});

const page = { origin: "https://vbloher.github.io", protocol: "https:", host: "vbloher.github.io" };
const devPage = { origin: "http://localhost:3000", protocol: "http:", host: "localhost:3000" };

describe("apiOrigin", () => {
  it.each([
    [undefined, ""],
    ["", ""],
    ["  ", ""],
    ["https://desk.example.com", "https://desk.example.com"],
    ["https://desk.example.com/", "https://desk.example.com"],
    ["https://desk.example.com:8443/api/v1", "https://desk.example.com:8443"],
    ["http://localhost:8080", "http://localhost:8080"],
  ])("reads VITE_API_URL %j as the origin %j", (value, origin) => {
    expect(apiOrigin(value)).toBe(origin);
  });

  it.each(["desk.example.com", "ftp://desk.example.com", "/api"])("refuses %j", (value) => {
    expect(() => apiOrigin(value)).toThrow(/VITE_API_URL/);
  });
});

describe("apiBaseUrl", () => {
  it("uses the page's own origin when no gateway is configured", () => {
    expect(apiBaseUrl("", devPage)).toBe("http://localhost:3000/api/v1");
  });

  it("uses the configured gateway otherwise", () => {
    expect(apiBaseUrl("https://desk.example.com", page)).toBe("https://desk.example.com/api/v1");
    expect(apiBaseUrl("http://localhost:8080", page)).toBe("http://localhost:8080/api/v1");
  });
});

describe("eventsUrl", () => {
  it("follows the page's scheme and host when no gateway is configured", () => {
    expect(eventsUrl("", devPage)).toBe("ws://localhost:3000/api/v1/ws");
    expect(eventsUrl("", page)).toBe("wss://vbloher.github.io/api/v1/ws");
  });

  it("follows the gateway's scheme and host otherwise", () => {
    expect(eventsUrl("https://desk.example.com", page)).toBe("wss://desk.example.com/api/v1/ws");
    expect(eventsUrl("http://localhost:8080", page)).toBe("ws://localhost:8080/api/v1/ws");
  });
});

describe("deskMissing", () => {
  it("is true only for a Pages build without a gateway", () => {
    expect(deskMissing("pages", "")).toBe(true);
    expect(deskMissing("pages", "https://desk.example.com")).toBe(false);
    expect(deskMissing(undefined, "")).toBe(false);
    expect(deskMissing("", "")).toBe(false);
  });
});

describe("base and router basename", () => {
  it.each([
    [undefined, "/"],
    ["", "/"],
    ["/", "/"],
    ["/canton-private-otc-desk/", "/canton-private-otc-desk/"],
    ["/canton-private-otc-desk", "/canton-private-otc-desk/"],
    ["canton-private-otc-desk", "/canton-private-otc-desk/"],
  ])("normalizes VITE_BASE %j to %j", (value, base) => {
    expect(normalizeBase(value)).toBe(base);
  });

  it("drops the trailing slash for the router", () => {
    expect(routerBasename("/canton-private-otc-desk/")).toBe("/canton-private-otc-desk");
    expect(routerBasename("/")).toBe("/");
  });
});
