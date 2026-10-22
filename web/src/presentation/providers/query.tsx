/**
 * The query cache. Every failed mutation shows a toast with the API's own
 * error text; a failed read toasts once (deduplicated by query), except quiet
 * background polls. A 401 anywhere means the session ended: `me` is re-read
 * and the desk guard sends the user to sign in.
 */
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";
import { errorMessage, isApiFailure } from "@/application/errors";
import { queryKeys } from "@/application/query-keys";

declare module "@tanstack/react-query" {
  interface Register {
    queryMeta: {
      /** A background poll: its failures stay silent (the socket badge and retries cover them). */
      quiet?: boolean;
    };
  }
}

export function QueryProvider({ children }: { children: ReactNode }) {
  const [client] = useState(() => {
    const sessionEnded = (e: unknown): boolean => isApiFailure(e) && e.status === 401;
    const qc: QueryClient = new QueryClient({
      queryCache: new QueryCache({
        onError: (e, query) => {
          if (sessionEnded(e)) {
            void qc.invalidateQueries({ queryKey: queryKeys.me });
            return;
          }
          if (query.meta?.quiet === true) return;
          toast.error(errorMessage(e), { id: `query:${query.queryHash}` });
        },
      }),
      mutationCache: new MutationCache({
        onError: (e) => {
          if (sessionEnded(e)) void qc.invalidateQueries({ queryKey: queryKeys.me });
          toast.error(errorMessage(e));
        },
      }),
      defaultOptions: {
        queries: { retry: 1, staleTime: 2_000, refetchOnWindowFocus: true },
        mutations: { retry: false },
      },
    });
    return qc;
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
