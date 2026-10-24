/**
 * Build-time pieces for static hosting (GitHub Pages): a 404.html that is the
 * app itself, so deep links load the SPA, and the Content-Security-Policy as a
 * meta tag, since Pages sends no headers of ours. The policy is the one nginx
 * sends (web/docker/nginx.conf.template), opened up to the gateway's origin.
 */
import type { Plugin } from "vite";

export interface PolicyOptions {
  /** The gateway's origin when it is not the page's own (config.apiOrigin), else "". */
  apiOrigin: string;
  /** For a <meta> policy: browsers ignore frame-ancestors there, so it is left out. */
  meta?: boolean;
}

export function contentSecurityPolicy({ apiOrigin, meta = false }: PolicyOptions): string {
  const connect = ["'self'"];
  if (apiOrigin !== "") {
    const { protocol, host } = new URL(apiOrigin);
    connect.push(apiOrigin, `${protocol === "https:" ? "wss" : "ws"}://${host}`);
  }
  const directives = [
    "default-src 'self'",
    "script-src 'self'",
    // React style attributes, sonner and Radix inject inline styles.
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data:",
    `connect-src ${connect.join(" ")}`,
    "base-uri 'none'",
    ...(meta ? [] : ["frame-ancestors 'none'"]),
    "form-action 'self'",
  ];
  return directives.join("; ");
}

/** Emits 404.html as a copy of the built index.html: static hosts serve it for unknown paths. */
export function spaFallback404(): Plugin {
  return {
    name: "otc-desk:spa-404",
    apply: "build",
    enforce: "post",
    generateBundle(_options, bundle) {
      const index = bundle["index.html"];
      if (index?.type !== "asset") return;
      this.emitFile({ type: "asset", fileName: "404.html", source: index.source });
    },
  };
}

/** Puts `policy` first in the head as a CSP meta tag, ahead of the scripts and styles it governs. */
export function cspMeta(policy: string): Plugin {
  return {
    name: "otc-desk:csp-meta",
    apply: "build", // the dev server's inline React refresh preamble would not pass it
    transformIndexHtml(html) {
      return {
        html,
        tags: [{ tag: "meta", attrs: { "http-equiv": "Content-Security-Policy", content: policy }, injectTo: "head-prepend" }],
      };
    },
  };
}
