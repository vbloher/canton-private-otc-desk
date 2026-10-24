/**
 * The SPA: providers (ports → query cache → auto-signer → session → events →
 * signer feed) around the routes. /signin is public; /trade, /wallet and
 * /activity need a session.
 */
import { BrowserRouter, Navigate, Route, Routes } from "react-router";
import type { Services } from "@/application/ports";
import { Toaster } from "@/presentation/components/ui/sonner";
import { DeskLayout } from "@/presentation/layout/desk-layout";
import { ActivityPage } from "@/presentation/pages/activity";
import { SignInPage } from "@/presentation/pages/signin";
import { TradePage } from "@/presentation/pages/trade";
import { WalletPage } from "@/presentation/pages/wallet";
import { AutoSignerProvider } from "@/presentation/providers/auto-signer";
import { EventsProvider } from "@/presentation/providers/events";
import { QueryProvider } from "@/presentation/providers/query";
import { ServicesProvider } from "@/presentation/providers/services";
import { SessionProvider } from "@/presentation/providers/session";
import { SignerProvider } from "@/presentation/providers/signer";

/** `basename`: the path the app is served under ("/" or e.g. "/canton-private-otc-desk" on Pages). */
export function App({ services, basename = "/" }: { services: Services; basename?: string }) {
  return (
    <ServicesProvider services={services}>
      <QueryProvider>
        <AutoSignerProvider>
          <SessionProvider>
            <EventsProvider>
              <SignerProvider>
                <BrowserRouter basename={basename}>
                  <Routes>
                    <Route path="/signin" element={<SignInPage />} />
                    <Route element={<DeskLayout />}>
                      <Route path="/trade" element={<TradePage />} />
                      <Route path="/wallet" element={<WalletPage />} />
                      <Route path="/activity" element={<ActivityPage />} />
                    </Route>
                    <Route path="/" element={<Navigate to="/trade" replace />} />
                    <Route path="*" element={<Navigate to="/" replace />} />
                  </Routes>
                </BrowserRouter>
                <Toaster position="top-right" closeButton />
              </SignerProvider>
            </EventsProvider>
          </SessionProvider>
        </AutoSignerProvider>
      </QueryProvider>
    </ServicesProvider>
  );
}
