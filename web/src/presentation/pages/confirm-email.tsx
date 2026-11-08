import { useQuery } from "@tanstack/react-query";
import { CircleCheck, Loader2, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useLocation } from "react-router";
import { tokenFromHash } from "@/application/confirmation-link";
import { Logo } from "@/presentation/components/desk/logo";
import { Button } from "@/presentation/components/ui/button";
import { Card, CardContent } from "@/presentation/components/ui/card";
import { useServices } from "@/presentation/providers/services";

export function ConfirmEmailPage() {
  const { profile } = useServices();
  const token = tokenFromHash(useLocation().hash);
  const confirm = useQuery({
    queryKey: ["confirm-email", token],
    queryFn: async () => {
      await profile.confirmEmail(token);
      return true;
    },
    enabled: token !== "",
    retry: false,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    meta: { quiet: true },
  });
  const failed = token === "" || confirm.isError;

  return (
    <main className="flex min-h-screen items-center justify-center p-6">
      <div className="w-full max-w-md space-y-4">
        <div className="flex items-center gap-3 pb-2">
          <Logo className="size-10" />
          <h1 className="text-xl font-semibold tracking-tight">Private OTC Desk</h1>
        </div>
        <Card>
          <CardContent>
            {failed ? (
              <Outcome
                icon={<TriangleAlert className="size-5 text-destructive" />}
                title="This link doesn't work"
                text="It may have expired, or a newer link replaced it. Open your profile and send a new one."
              />
            ) : confirm.isSuccess ? (
              <Outcome
                icon={<CircleCheck className="size-5 text-success" />}
                title="Email confirmed"
                text="You can now choose what the desk sends you."
              />
            ) : (
              <p className="flex items-center gap-2 text-muted-foreground" role="status">
                <Loader2 className="size-4 animate-spin" /> Confirming your email…
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </main>
  );
}

function Outcome({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return (
    <div className="space-y-3" role="status">
      <p className="flex items-center gap-2 text-base font-medium">
        {icon} {title}
      </p>
      <p className="text-muted-foreground">{text}</p>
      <Button asChild variant="outline">
        <Link to="/profile">Open profile</Link>
      </Button>
    </div>
  );
}
