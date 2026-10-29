import { useMutation } from "@tanstack/react-query";
import { ArrowRight, Lock, LogOut } from "lucide-react";
import { Navigate, Outlet, useLocation, useNavigate } from "react-router";
import { toast } from "sonner";
import { Notice } from "@/presentation/components/desk/notice";
import { Button } from "@/presentation/components/ui/button";
import { useUnlock } from "@/presentation/hooks/queries";
import { useDeskEvent } from "@/presentation/providers/events";
import { useSession } from "@/presentation/providers/session";
import { useSigner } from "@/presentation/providers/signer";
import { Sidebar } from "./sidebar";

const TITLES: Record<string, string> = {
  "/trade": "Trade",
  "/wallet": "Wallet",
  "/activity": "Activity",
};

export function DeskLayout() {
  const session = useSession();
  if (session.status === "loading") {
    return <main className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">Loading…</main>;
  }
  if (session.status === "error") {
    return (
      <main className="flex min-h-screen items-center justify-center p-6">
        <div className="w-full max-w-md">
          <Notice>
            <p>Can&apos;t reach the desk right now.</p>
            <Button variant="outline" size="sm" onClick={session.retry}>
              Try again
            </Button>
          </Notice>
        </div>
      </main>
    );
  }
  if (session.status === "signed-out") return <Navigate to="/signin" replace />;
  return <Desk />;
}

function Desk() {
  useTradeToasts();
  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <Header />
        <LockedBanner />
        <main className="flex min-h-0 flex-1 flex-col p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

function Header() {
  const { me, logout } = useSession();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const leave = useMutation({
    mutationFn: logout,
    onSettled: () => void navigate("/signin", { replace: true }),
  });
  if (me === null) return null;
  return (
    <header className="flex items-center justify-between gap-4 border-b border-border px-6 py-3">
      <h1 className="text-xl font-semibold tracking-tight">{TITLES[pathname] ?? ""}</h1>
      <div className="flex items-center gap-3">
        <div className="max-w-56 text-right leading-tight">
          <p className="truncate text-sm font-medium">{me.displayName}</p>
          <p className="tabular truncate text-[0.6875rem] text-muted-foreground" title={me.partyId}>
            {me.hint}
          </p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="text-muted-foreground hover:text-foreground"
          disabled={leave.isPending}
          onClick={() => leave.mutate()}
        >
          <LogOut />
          Log out
        </Button>
      </div>
    </header>
  );
}

function LockedBanner() {
  const { locked } = useSession();
  const { pending } = useSigner();
  const { unlock, unlocking } = useUnlock();
  if (!locked) return null;
  return (
    <div
      role="status"
      className="flex items-center justify-between gap-3 border-b border-primary/50 bg-primary/10 px-6 py-2.5 text-sm"
    >
      <p className="flex min-w-0 items-center gap-2">
        <Lock className="size-4 flex-none text-primary" />
        <span className="font-medium">Signing is locked · {pending.length} pending</span>
        <span className="hidden truncate text-muted-foreground md:inline">
          — unlock with your passkey to quote, trade and accept transfers.
        </span>
      </p>
      <Button size="sm" disabled={unlocking} onClick={() => unlock()}>
        {unlocking ? "Unlocking…" : "Unlock"}
        {!unlocking && <ArrowRight />}
      </Button>
    </div>
  );
}

function useTradeToasts(): void {
  const navigate = useNavigate();
  useDeskEvent((event) => {
    if (event.type !== "trade.updated") return;
    const { tradeId, status } = event.data;
    if (status === "settled") {
      toast.success("Trade settled", {
        id: `trade:${tradeId}:settled`,
        description: "Both legs moved in one atomic Canton transaction.",
        action: { label: "Activity", onClick: () => void navigate("/activity") },
      });
    } else if (status === "failed") {
      toast.error("Settlement failed", {
        id: `trade:${tradeId}:failed`,
        description: "Nothing moved. If the request is still open, another quote can be accepted.",
      });
    }
  });
}
