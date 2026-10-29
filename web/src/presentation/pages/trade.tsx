import type { ReactNode } from "react";
import { useSearchParams } from "react-router";
import { incomingRowStatus, rfqRowStatus } from "@/domain/rfq";
import type { IncomingRfq, RfqWithQuotes } from "@/domain/types";
import { Notice } from "@/presentation/components/desk/notice";
import { IncomingRfqDetail, MyRfqDetail } from "@/presentation/components/desk/rfq-detail";
import { IncomingRfqList, MyRfqList } from "@/presentation/components/desk/rfq-list";
import { Button } from "@/presentation/components/ui/button";
import { Skeleton } from "@/presentation/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/presentation/components/ui/tabs";
import { useIncomingRfqs, useMyRfqs } from "@/presentation/hooks/queries";
import { useNow } from "@/presentation/hooks/use-now";
import { useSession } from "@/presentation/providers/session";

type Tab = "mine" | "incoming";

const newestFirst = (a: { rfq: { createdAt: number } }, b: { rfq: { createdAt: number } }): number =>
  b.rfq.createdAt - a.rfq.createdAt;

function sortMine(entries: RfqWithQuotes[], now: number): RfqWithQuotes[] {
  const rank = (e: RfqWithQuotes): number => {
    const s = rfqRowStatus(e, now);
    return s === "open" || s === "settling" ? 0 : 1;
  };
  return [...entries].sort((a, b) => rank(a) - rank(b) || newestFirst(a, b));
}

function sortIncoming(entries: IncomingRfq[], now: number): IncomingRfq[] {
  const rank = (e: IncomingRfq): number => {
    const s = incomingRowStatus(e, now);
    return s === "new" ? 0 : s === "allocating" || s === "open" || s === "accepted" ? 1 : 2;
  };
  return [...entries].sort((a, b) => rank(a) - rank(b) || newestFirst(a, b));
}

export function TradePage() {
  const [params, setParams] = useSearchParams();
  const tab: Tab = params.get("tab") === "incoming" ? "incoming" : "mine";
  const selectedId = params.get("rfq") ?? undefined;
  const { me } = useSession();
  const now = useNow();
  const mine = useMyRfqs();
  const incoming = useIncomingRfqs();

  const mineList = sortMine(mine.data ?? [], now);
  const incomingList = sortIncoming(incoming.data ?? [], now);
  const mineSelected = mineList.find((e) => e.rfq.id === selectedId) ?? mineList[0];
  const incomingSelected = incomingList.find((e) => e.rfq.id === selectedId) ?? incomingList[0];
  const waitingForMe = incomingList.some((e) => incomingRowStatus(e, now) === "new");

  const select = (next: Tab, rfq?: string): void =>
    setParams(rfq === undefined ? { tab: next } : { tab: next, rfq }, { replace: true });

  return (
    <Tabs value={tab} onValueChange={(v) => select(v === "incoming" ? "incoming" : "mine")} className="min-h-0 flex-1">
      <TabsList>
        <TabsTrigger value="mine">My requests · {mineList.length}</TabsTrigger>
        <TabsTrigger value="incoming">
          Incoming · {incomingList.length}
          {waitingForMe && (
            <>
              <span aria-hidden className="text-primary">
                ●
              </span>
              <span className="sr-only">, some wait for your quote</span>
            </>
          )}
        </TabsTrigger>
      </TabsList>

      <TabsContent value="mine">
        <MasterDetail
          loading={mine.isPending}
          failed={mine.isError && mine.data === undefined}
          retry={() => void mine.refetch()}
          empty={mineList.length === 0}
          emptyText="No requests yet. Use “Request quotes” — only the makers you pick will see it."
          list={<MyRfqList entries={mineList} selectedId={mineSelected?.rfq.id} onSelect={(id) => select("mine", id)} />}
          detail={mineSelected === undefined ? null : <MyRfqDetail key={mineSelected.rfq.id} entry={mineSelected} />}
        />
      </TabsContent>

      <TabsContent value="incoming">
        <MasterDetail
          loading={incoming.isPending}
          failed={incoming.isError && incoming.data === undefined}
          retry={() => void incoming.refetch()}
          empty={incomingList.length === 0}
          emptyText={
            me?.makerMode === true
              ? "No requests for you right now. Takers who ask you appear here instantly."
              : "Turn on “Maker mode” in the sidebar to be listed as a maker takers can ask."
          }
          list={
            <IncomingRfqList
              entries={incomingList}
              selectedId={incomingSelected?.rfq.id}
              onSelect={(id) => select("incoming", id)}
            />
          }
          detail={
            incomingSelected === undefined ? null : (
              <IncomingRfqDetail key={incomingSelected.rfq.id} entry={incomingSelected} />
            )
          }
        />
      </TabsContent>
    </Tabs>
  );
}

function MasterDetail({
  loading,
  failed,
  retry,
  empty,
  emptyText,
  list,
  detail,
}: {
  loading: boolean;
  failed: boolean;
  retry: () => void;
  empty: boolean;
  emptyText: string;
  list: ReactNode;
  detail: ReactNode;
}) {
  if (loading) {
    return (
      <div className="grid gap-4 lg:grid-cols-[2fr_3fr]">
        <div className="space-y-2">
          <Skeleton className="h-16" />
          <Skeleton className="h-16" />
        </div>
        <Skeleton className="h-64" />
      </div>
    );
  }
  if (failed) {
    return (
      <Notice>
        <p>Could not load the requests.</p>
        <Button variant="outline" size="sm" onClick={retry}>
          Try again
        </Button>
      </Notice>
    );
  }
  if (empty) return <Notice>{emptyText}</Notice>;
  return (
    <div className="grid min-h-0 gap-4 lg:grid-cols-[2fr_3fr]">
      <div className="space-y-2 overflow-y-auto pr-1">{list}</div>
      <div className="overflow-y-auto rounded-lg border border-border bg-card p-5">{detail}</div>
    </div>
  );
}
