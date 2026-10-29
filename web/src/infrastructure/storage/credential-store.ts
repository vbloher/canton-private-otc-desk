import type { CredentialStore } from "@/application/ports";

const KEY = "otc-desk:credentials";

export function createCredentialStore(storage: Storage | undefined): CredentialStore {
  function read(): Record<string, string> {
    try {
      const parsed: unknown = JSON.parse(storage?.getItem(KEY) ?? "{}");
      return typeof parsed === "object" && parsed !== null ? (parsed as Record<string, string>) : {};
    } catch {
      return {};
    }
  }

  return {
    get: (userId) => {
      const id = read()[userId];
      return typeof id === "string" ? id : undefined;
    },
    set: (userId, credentialId) => {
      try {
        storage?.setItem(KEY, JSON.stringify({ ...read(), [userId]: credentialId }));
      } catch {
        return;
      }
    },
  };
}
