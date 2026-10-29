import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import type { Services } from "@/application/ports";
import { createHttpApis } from "@/infrastructure/api/http-apis";
import { ed25519Keys } from "@/infrastructure/crypto/ed25519";
import { browserWebAuthn, createPasskeyVault } from "@/infrastructure/crypto/prf";
import { createCredentialStore } from "@/infrastructure/storage/credential-store";
import { createEventsSource, deskEventsUrl } from "@/infrastructure/ws/events-socket";
import { App } from "@/presentation/app";
import "./index.css";

function localStorageOrNothing(): Storage | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

const services: Services = {
  ...createHttpApis({ baseUrl: `${window.location.origin}/api/v1` }),
  vault: createPasskeyVault(browserWebAuthn()),
  keys: ed25519Keys,
  credentials: createCredentialStore(localStorageOrNothing()),
  events: createEventsSource({ url: () => deskEventsUrl(window.location) }),
};

const root = document.getElementById("root");
if (root === null) throw new Error("index.html has no #root");
createRoot(root).render(
  <StrictMode>
    <App services={services} />
  </StrictMode>,
);
