/**
 * Composition root: the only module that knows every layer. It reads the
 * build-time settings, builds the infrastructure adapters for the application's
 * ports and hands them to the presentation.
 */
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import type { Services } from "@/application/ports";
import { createHttpApis } from "@/infrastructure/api/http-apis";
import { createSessionTokens } from "@/infrastructure/api/session-token";
import {
  apiBaseUrl,
  apiOrigin,
  deskMissing,
  eventsUrl,
  framingRefused,
  routerBasename,
  sessionTransport,
} from "@/infrastructure/config";
import { ed25519Keys } from "@/infrastructure/crypto/ed25519";
import { browserWebAuthn, createPasskeyVault } from "@/infrastructure/crypto/prf";
import { createCredentialStore } from "@/infrastructure/storage/credential-store";
import { createEventsSource, eventProtocols } from "@/infrastructure/ws/events-socket";
import { App } from "@/presentation/app";
import { FramedNotice } from "@/presentation/pages/framed";
import { NoDeskPage } from "@/presentation/pages/no-desk";
import "./index.css";

/** Web storage, or nothing where the browser refuses it (then sign-in lasts as long as the page). */
function storage(kind: "localStorage" | "sessionStorage"): Storage | undefined {
  try {
    return window[kind];
  } catch {
    return undefined;
  }
}

const origin = apiOrigin(import.meta.env.VITE_API_URL);
const deploy = import.meta.env.VITE_DEPLOY;

function services(): Services {
  // On the page's own origin the session stays the HttpOnly cookie: no token ever reaches JavaScript.
  const tokens = sessionTransport(origin) === "bearer" ? createSessionTokens(storage("sessionStorage")) : undefined;
  return {
    ...createHttpApis({ baseUrl: apiBaseUrl(origin, window.location), tokens }),
    vault: createPasskeyVault(browserWebAuthn()),
    keys: ed25519Keys,
    credentials: createCredentialStore(storage("localStorage")),
    events: createEventsSource({
      url: () => eventsUrl(origin, window.location),
      protocols: () => eventProtocols(tokens?.get()),
    }),
  };
}

const root = document.getElementById("root");
if (root === null) throw new Error("index.html has no #root");
createRoot(root).render(
  <StrictMode>
    {framingRefused(deploy, window) ? (
      <FramedNotice />
    ) : deskMissing(deploy, origin) ? (
      <NoDeskPage />
    ) : (
      <App services={services()} basename={routerBasename(import.meta.env.BASE_URL)} />
    )}
  </StrictMode>,
);
