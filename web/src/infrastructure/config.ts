/**
 * Where the desk is, decided at build time. VITE_API_URL is the gateway's origin
 * (e.g. https://desk.example.com) for a deployment on another origin, such as
 * GitHub Pages; empty means the page's own origin (the dev proxy, nginx).
 * Pure functions only: vite.config.ts uses them too.
 */

/** The parts of `window.location` the URLs are built from. */
export interface PageLocation {
  origin: string;
  protocol: string;
  host: string;
}

/** VITE_API_URL → the gateway's origin, or "" for the page's own. Anything but an absolute http(s) URL is an error. */
export function apiOrigin(value: string | undefined): string {
  const text = (value ?? "").trim();
  if (text === "") return "";
  let url: URL;
  try {
    url = new URL(text);
  } catch {
    throw new Error(`VITE_API_URL must be an absolute http(s) URL such as https://desk.example.com, got ${JSON.stringify(text)}`);
  }
  if (url.protocol !== "https:" && url.protocol !== "http:") {
    throw new Error(`VITE_API_URL must be an http(s) URL, got ${JSON.stringify(text)}`);
  }
  return url.origin;
}

/** The REST base of the desk: `<origin>/api/v1`. */
export function apiBaseUrl(origin: string, page: PageLocation): string {
  return `${origin === "" ? page.origin : origin}/api/v1`;
}

/** The event socket: ws(s)://<gateway or page host>/api/v1/ws, secure when the HTTP side is. */
export function eventsUrl(origin: string, page: PageLocation): string {
  const { protocol, host } = origin === "" ? page : new URL(origin);
  return `${protocol === "https:" ? "wss" : "ws"}://${host}/api/v1/ws`;
}

/**
 * How the session travels. On the page's own origin it stays the HttpOnly cookie,
 * out of JavaScript's reach. Across origins the cookie would be a third-party
 * cookie, so the token sign-in answers with is kept (this tab only) and sent as
 * a Bearer.
 */
export type SessionTransport = "cookie" | "bearer";

export function sessionTransport(origin: string): SessionTransport {
  return origin === "" ? "cookie" : "bearer";
}

/**
 * The desk must not run inside another page: its auto-signer signs on its own.
 * nginx forbids framing with frame-ancestors; a Pages build cannot send that
 * header (a meta policy cannot carry it), so it checks for itself. A top window
 * that cannot even be inspected counts as framed.
 */
export function framingRefused(deploy: string | undefined, win: { readonly top: unknown; readonly self: unknown }): boolean {
  if (deploy !== "pages") return false;
  try {
    return win.top !== win.self;
  } catch {
    return true;
  }
}

const LOOPBACK_HOSTS = new Set(["localhost", "127.0.0.1", "[::1]"]);

/**
 * A Pages site is served over https, so a browser blocks calls to a plain-http
 * gateway as mixed content — except on loopback. Fails the build instead.
 */
export function checkPagesApiOrigin(deploy: string | undefined, origin: string): void {
  if (deploy !== "pages" || origin === "") return;
  const { protocol, hostname } = new URL(origin);
  if (protocol === "http:" && !LOOPBACK_HOSTS.has(hostname)) {
    throw new Error(
      `VITE_API_URL must be an https:// origin for a Pages build: the site is served over https, so browsers would block ${origin} as mixed content (plain http is accepted only for localhost, 127.0.0.1 and [::1]).`,
    );
  }
}

/** A Pages build has no desk of its own: without a configured gateway there is nothing to talk to. */
export function deskMissing(deploy: string | undefined, origin: string): boolean {
  return deploy === "pages" && origin === "";
}

/** VITE_BASE → Vite's `base`, with a leading and a trailing slash ("/" by default). */
export function normalizeBase(value: string | undefined): string {
  const path = (value ?? "").trim().replace(/^\/+|\/+$/g, "");
  return path === "" ? "/" : `/${path}/`;
}

/** BrowserRouter's basename from Vite's BASE_URL: "/repo/" → "/repo". */
export function routerBasename(baseUrl: string): string {
  return baseUrl === "/" ? "/" : baseUrl.replace(/\/+$/, "");
}
