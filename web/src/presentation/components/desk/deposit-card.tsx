/** Where others send tokens: this wallet's full Canton party id. */
import { Card, CardContent, CardHeader, CardTitle } from "@/presentation/components/ui/card";
import { useSession } from "@/presentation/providers/session";
import { CopyButton } from "./copy-button";

export function DepositCard() {
  const { me } = useSession();
  if (me === null) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Deposit</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        <p className="label-caps">Your party id</p>
        <div className="flex items-center gap-2">
          <p className="tabular min-w-0 flex-1 rounded-md border border-border bg-secondary p-3 text-xs break-all">
            {me.partyId}
          </p>
          <CopyButton value={me.partyId} label="Copy party id" />
        </div>
        <p className="text-sm text-muted-foreground">
          Any party on this Canton network can send you tokens here. A transfer lands below as an offer and is yours once
          you accept it — accepting is a signature.
        </p>
      </CardContent>
    </Card>
  );
}
