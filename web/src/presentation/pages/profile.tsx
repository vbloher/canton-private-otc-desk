import { CircleCheck, Loader2, MailCheck } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import type { NotificationPreferences, Profile } from "@/domain/types";
import { Notice } from "@/presentation/components/desk/notice";
import { Button } from "@/presentation/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/presentation/components/ui/card";
import { Checkbox } from "@/presentation/components/ui/checkbox";
import { Input } from "@/presentation/components/ui/input";
import { Label } from "@/presentation/components/ui/label";
import { useProfile, useSetEmail, useSetNotifications } from "@/presentation/hooks/queries";

export function ProfilePage() {
  const profile = useProfile();

  if (profile.isPending) {
    return <p className="text-sm text-muted-foreground">Loading your profile…</p>;
  }
  if (profile.isError) {
    return (
      <Notice>
        <p>Can&apos;t load your profile right now.</p>
        <Button variant="outline" size="sm" onClick={() => void profile.refetch()}>
          Try again
        </Button>
      </Notice>
    );
  }
  if (!profile.data.emailEnabled) {
    return <Notice>Email isn&apos;t set up on this desk.</Notice>;
  }

  return (
    <div className="mx-auto w-full max-w-2xl space-y-6">
      <EmailCard profile={profile.data} />
      <NotificationsCard profile={profile.data} />
    </div>
  );
}

function EmailCard({ profile }: { profile: Profile }) {
  const setEmail = useSetEmail();
  const [draft, setDraft] = useState<string | null>(null);
  const saved = profile.email ?? "";
  const value = draft ?? saved;
  const address = value.trim();
  const edited = address.toLowerCase() !== saved.toLowerCase();
  const canSend = address !== "" && edited && !setEmail.isPending;

  const send = (email: string): void => {
    setEmail.mutate(email, {
      onSuccess: () => {
        setDraft(null);
        toast.success("Confirmation link sent", { description: `Check ${email}.` });
      },
    });
  };
  const submit = (e: FormEvent): void => {
    e.preventDefault();
    if (canSend) send(address);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Email</CardTitle>
        <CardDescription>Used only for the notifications you choose below.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <form className="space-y-1.5" onSubmit={submit}>
          <Label htmlFor="email">Email address</Label>
          <div className="flex gap-2">
            <Input
              id="email"
              type="email"
              value={value}
              onChange={(e) => setDraft(e.target.value)}
              placeholder="you@example.com"
              autoComplete="email"
              maxLength={254}
              required
            />
            <Button type="submit" size="lg" disabled={!canSend}>
              {setEmail.isPending ? (
                <>
                  <Loader2 className="animate-spin" /> Sending…
                </>
              ) : (
                "Confirm email"
              )}
            </Button>
          </div>
        </form>
        {!edited && profile.emailStatus === "confirmed" && (
          <p className="flex items-center gap-2 font-medium text-success" role="status">
            <CircleCheck className="size-4 flex-none" /> Email confirmed
          </p>
        )}
        {!edited && profile.emailStatus === "unconfirmed" && (
          <div className="flex flex-wrap items-center justify-between gap-2" role="status">
            <p className="flex min-w-0 items-center gap-2 text-muted-foreground">
              <MailCheck className="size-4 flex-none text-primary" />
              <span className="min-w-0 break-words">We sent a link to {saved}. Open it to confirm this address.</span>
            </p>
            <Button variant="outline" size="sm" disabled={setEmail.isPending} onClick={() => send(saved)}>
              Send again
            </Button>
          </div>
        )}
        {edited && profile.emailStatus === "confirmed" && (
          <p className="text-muted-foreground">Changing it stops your emails until you confirm the new address.</p>
        )}
      </CardContent>
    </Card>
  );
}

const CHOICES: { key: keyof NotificationPreferences; title: string; text: string }[] = [
  {
    key: "trades",
    title: "My trades",
    text: "An email when your trade settles, with what you gave and received. It never names the other side.",
  },
  { key: "news", title: "News", text: "Project news and product updates." },
];

function NotificationsCard({ profile }: { profile: Profile }) {
  const save = useSetNotifications();
  const confirmed = profile.emailStatus === "confirmed";
  const current: NotificationPreferences =
    save.isPending ? save.variables : { news: profile.news, trades: profile.trades };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Notifications</CardTitle>
        <CardDescription>
          {confirmed ? "Choose what the desk sends to your email." : "Confirm your email to choose what you receive."}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {CHOICES.map(({ key, title, text }) => (
          <div key={key} className="flex items-start gap-3">
            <Checkbox
              id={`notify-${key}`}
              className="mt-0.5"
              checked={confirmed && current[key]}
              disabled={!confirmed || save.isPending}
              onCheckedChange={(checked) => save.mutate({ ...current, [key]: checked === true })}
            />
            <div className="space-y-1">
              <Label htmlFor={`notify-${key}`}>{title}</Label>
              <p className="text-muted-foreground">{text}</p>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}
