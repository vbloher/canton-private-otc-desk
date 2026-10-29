import { Activity, ArrowLeftRight, Wallet } from "lucide-react";
import { NavLink } from "react-router";
import type { SocketStatus } from "@/application/ports";
import type { NetworkName } from "@/domain/types";
import { Logo } from "@/presentation/components/desk/logo";
import { NewRfqDialog } from "@/presentation/components/desk/new-rfq-dialog";
import { Switch } from "@/presentation/components/ui/switch";
import { useNetwork, useSetMakerMode } from "@/presentation/hooks/queries";
import { cn } from "@/presentation/lib/utils";
import { useSocketStatus } from "@/presentation/providers/events";
import { useSession } from "@/presentation/providers/session";

const NAV = [
  { to: "/trade", label: "Trade", icon: ArrowLeftRight },
  { to: "/wallet", label: "Wallet", icon: Wallet },
  { to: "/activity", label: "Activity", icon: Activity },
] as const;

const NETWORK: Record<NetworkName, string> = {
  devnet: "DevNet",
  testnet: "TestNet",
  mainnet: "MainNet",
};

const SOCKET: Record<SocketStatus, { label: string; dot: string }> = {
  live: { label: "Live", dot: "bg-success" },
  connecting: { label: "Connecting…", dot: "animate-pulse bg-primary" },
  offline: { label: "Offline — retrying", dot: "bg-destructive" },
};

export function Sidebar() {
  const { me } = useSession();
  const socket = SOCKET[useSocketStatus()];
  const network = useNetwork().data;
  const makerMode = useSetMakerMode();
  const checked = makerMode.isPending ? makerMode.variables : (me?.makerMode ?? false);

  return (
    <aside className="sticky top-0 flex h-screen w-56 flex-none flex-col overflow-y-auto border-r border-sidebar-border bg-sidebar text-sidebar-foreground">
      <div className="flex items-center gap-2.5 px-4 py-5">
        <Logo className="size-7 flex-none" />
        <div className="leading-tight">
          <p className="font-semibold tracking-tight">Private OTC Desk</p>
          <p className="text-[0.6875rem] text-muted-foreground">
            {network === undefined ? "Canton" : `Canton · ${NETWORK[network.name]}`}
          </p>
        </div>
      </div>

      <div className="px-3 pb-3">
        <NewRfqDialog />
      </div>

      <nav className="flex flex-col gap-1 px-2" aria-label="Desk">
        {NAV.map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            className={({ isActive }) =>
              cn(
                "flex items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors",
                isActive
                  ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                  : "text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
              )
            }
          >
            <Icon className="size-4" />
            {label}
          </NavLink>
        ))}
      </nav>

      <div className="mt-auto space-y-3 px-3 pb-4">
        <p
          className="flex items-center gap-2 rounded-md border border-sidebar-border px-3 py-1.5 text-xs text-muted-foreground"
          role="status"
          aria-live="polite"
        >
          <span aria-hidden className={cn("size-2 rounded-full", socket.dot)} />
          {socket.label}
        </p>

        <label className="flex cursor-pointer items-center justify-between gap-2 rounded-md border border-sidebar-border bg-card px-3 py-2.5">
          <span className="leading-tight">
            <span className="block text-sm font-medium">Maker mode</span>
            <span className="block text-[0.6875rem] text-muted-foreground">Receive quote requests</span>
          </span>
          <Switch
            checked={checked}
            onCheckedChange={(v) => {
              if (!makerMode.isPending) makerMode.mutate(v);
            }}
            disabled={me === null}
          />
        </label>
      </div>
    </aside>
  );
}
