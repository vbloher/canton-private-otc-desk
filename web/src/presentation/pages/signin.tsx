/**
 * The door: connect an existing wallet with its passkey, or create one. Both
 * are passkey ceremonies — nothing to type but a display name.
 */
import { useMutation, useQuery } from "@tanstack/react-query";
import { KeyRound, Loader2, TriangleAlert } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Navigate, useNavigate } from "react-router";
import type { CreateStep } from "@/application/auth";
import { PrfUnsupportedError, UnknownPasskeyError } from "@/application/errors";
import { Logo } from "@/presentation/components/desk/logo";
import { Button } from "@/presentation/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/presentation/components/ui/card";
import { Input } from "@/presentation/components/ui/input";
import { Label } from "@/presentation/components/ui/label";
import { useServices } from "@/presentation/providers/services";
import { useSession } from "@/presentation/providers/session";

const STEP: Record<CreateStep, string> = {
  passkey: "Creating your passkey… (1 of 3)",
  unlock: "Confirm the passkey once more… (2 of 3)",
  party: "Creating your Canton party… (3 of 3)",
};

const NAME_MIN = 2;
const NAME_MAX = 32;

export function SignInPage() {
  const session = useSession();
  const navigate = useNavigate();
  const { vault } = useServices();
  const support = useQuery({
    queryKey: ["passkey-support"],
    queryFn: () => vault.support(),
    staleTime: Infinity,
    retry: false,
  });
  const [name, setName] = useState("");
  const [step, setStep] = useState<CreateStep>("passkey");

  const enter = (): void => void navigate("/trade", { replace: true });
  const create = useMutation({
    mutationFn: (displayName: string) => session.createWallet(displayName, setStep),
    onSuccess: enter,
  });
  const connect = useMutation({ mutationFn: () => session.connectWallet(), onSuccess: enter });

  if (session.status === "signed-in") return <Navigate to="/trade" replace />;

  const failure = connect.isError ? connect.error : create.isError ? create.error : null;
  const noPasskeys = support.data === "none";
  const noPrf = support.data === "no-prf" || failure instanceof PrfUnsupportedError;
  const busy = create.isPending || connect.isPending;
  const trimmed = name.trim();
  const nameOk = trimmed.length >= NAME_MIN && trimmed.length <= NAME_MAX;

  return (
    <main className="flex min-h-screen items-center justify-center p-6">
      <div className="w-full max-w-lg space-y-4">
        <div className="flex items-center gap-3 pb-2">
          <Logo className="size-10" />
          <div>
            <h1 className="text-xl font-semibold tracking-tight">Private OTC Desk</h1>
            <p className="text-sm text-muted-foreground">
              Request quotes from makers you choose; trades settle atomically on Canton.
            </p>
          </div>
        </div>

        {session.status === "error" && (
          <Alert>
            Can&apos;t reach the desk right now.{" "}
            <button type="button" className="underline underline-offset-4" onClick={session.retry}>
              Try again
            </button>
          </Alert>
        )}
        {noPasskeys && (
          <Alert>
            This browser can&apos;t use passkeys on this page. Open the desk over https (or on localhost) in a current
            Chrome, Edge or Safari.
          </Alert>
        )}
        {noPrf && (
          <Alert>
            {failure instanceof PrfUnsupportedError
              ? failure.message
              : "This browser's passkeys don't support the WebAuthn PRF extension, which the desk needs to protect your key. Use a current Chrome, Edge or Safari with a synced passkey."}
          </Alert>
        )}
        {failure instanceof UnknownPasskeyError && (
          <Alert>
            No wallet is registered for this passkey on this desk. Create a wallet below, or connect again and pick
            another passkey.
          </Alert>
        )}

        <Card>
          <CardHeader>
            <CardTitle>Connect wallet</CardTitle>
            <CardDescription>Sign in with the passkey that protects your wallet.</CardDescription>
          </CardHeader>
          <CardContent>
            <Button size="lg" className="w-full" disabled={busy || noPasskeys} onClick={() => connect.mutate()}>
              {connect.isPending ? (
                <>
                  <Loader2 className="animate-spin" /> Waiting for your passkey…
                </>
              ) : (
                <>
                  <KeyRound /> Connect with passkey
                </>
              )}
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Create wallet</CardTitle>
            <CardDescription>
              A new Canton party. Its key is generated in this browser and locked with a passkey — the desk only ever
              stores it encrypted.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="space-y-3"
              onSubmit={(e) => {
                e.preventDefault();
                if (nameOk && !busy) create.mutate(trimmed);
              }}
            >
              <div className="space-y-1.5">
                <Label htmlFor="display-name">Display name</Label>
                <Input
                  id="display-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Alice"
                  maxLength={NAME_MAX}
                  autoComplete="nickname"
                  disabled={busy}
                />
                <p className="text-xs text-muted-foreground">
                  Counterparties see this name, and your party hint is derived from it.
                </p>
              </div>
              <Button type="submit" variant="outline" size="lg" className="w-full" disabled={!nameOk || busy || noPasskeys}>
                {create.isPending ? (
                  <>
                    <Loader2 className="animate-spin" /> {STEP[step]}
                  </>
                ) : (
                  "Create wallet"
                )}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </main>
  );
}

/** Amber, like the desk's locked banner: something to act on, not a footnote. */
function Alert({ children }: { children: ReactNode }) {
  return (
    <div
      role="alert"
      className="flex items-start gap-3 rounded-lg border border-primary/50 bg-primary/10 px-4 py-3 text-sm font-medium"
    >
      <TriangleAlert className="mt-0.5 size-4 shrink-0 text-primary" />
      <p>{children}</p>
    </div>
  );
}
