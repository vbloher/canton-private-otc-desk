import type { Plugin } from "vite";
import { describe, expect, it } from "vitest";
import { contentSecurityPolicy, cspMeta, spaFallback404 } from "./pages";

/** Calls a plugin hook given as a plain function, with `this` set to `context`. */
function call(hook: Plugin[keyof Plugin], context: object, ...args: unknown[]): unknown {
  if (typeof hook !== "function") throw new Error("expected the hook as a function");
  return (hook as (...a: unknown[]) => unknown).apply(context, args);
}

describe("contentSecurityPolicy", () => {
  it("is nginx's policy for a desk on the page's own origin", () => {
    expect(contentSecurityPolicy({ apiOrigin: "" })).toBe(
      "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'",
    );
  });

  it("lets the page reach a gateway on another origin over HTTP and WebSocket", () => {
    expect(contentSecurityPolicy({ apiOrigin: "https://desk.example.com" })).toContain(
      "connect-src 'self' https://desk.example.com wss://desk.example.com;",
    );
    expect(contentSecurityPolicy({ apiOrigin: "http://localhost:8080" })).toContain(
      "connect-src 'self' http://localhost:8080 ws://localhost:8080;",
    );
  });

  it("leaves frame-ancestors out of a meta policy, where browsers do not honour it", () => {
    expect(contentSecurityPolicy({ apiOrigin: "https://desk.example.com", meta: true })).toBe(
      "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' https://desk.example.com wss://desk.example.com; base-uri 'none'; form-action 'self'",
    );
  });
});

describe("spaFallback404", () => {
  it("emits 404.html as a copy of the built index.html, so Pages serves the app on deep links", () => {
    const emitted: unknown[] = [];
    const bundle = { "index.html": { type: "asset", fileName: "index.html", source: "<html>app</html>" } };

    call(spaFallback404().generateBundle, { emitFile: (file: unknown) => emitted.push(file) }, {}, bundle, true);

    expect(emitted).toEqual([{ type: "asset", fileName: "404.html", source: "<html>app</html>" }]);
  });

  it("emits nothing when there is no index.html", () => {
    const emitted: unknown[] = [];

    call(spaFallback404().generateBundle, { emitFile: (file: unknown) => emitted.push(file) }, {}, {}, true);

    expect(emitted).toEqual([]);
  });
});

describe("cspMeta", () => {
  it("puts the policy first in the head, ahead of every script and style it governs", () => {
    const html = "<html><head></head><body></body></html>";

    expect(call(cspMeta("default-src 'self'").transformIndexHtml, {}, html)).toEqual({
      html,
      tags: [
        {
          tag: "meta",
          attrs: { "http-equiv": "Content-Security-Policy", content: "default-src 'self'" },
          injectTo: "head-prepend",
        },
      ],
    });
  });
});
