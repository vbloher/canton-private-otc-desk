/**
 * What a static deployment (GitHub Pages) shows when it was built without a
 * gateway: the page itself works, but there is no desk to trade against.
 */
import { BookOpen } from "lucide-react";
import { Logo } from "@/presentation/components/desk/logo";
import { Button } from "@/presentation/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/presentation/components/ui/card";

const REPOSITORY = "https://github.com/vbloher/canton-private-otc-desk";

export function NoDeskPage() {
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

        <Card>
          <CardHeader>
            <CardTitle>No desk is connected</CardTitle>
            <CardDescription>
              This static deployment needs a desk — the gateway of a running backend — and none was configured
              when it was built.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <p>
              <span className="font-medium">Connect one:</span> host the backend, set the repository variable{" "}
              <code className="tabular rounded bg-secondary px-1 py-0.5 text-xs">API_URL</code> to its gateway&apos;s
              origin and re-run the Pages workflow (
              <a className="text-primary underline-offset-4 hover:underline" href={`${REPOSITORY}#web-app-on-github-pages`}>
                how
              </a>
              ).
            </p>
            <p>
              <span className="font-medium">Or run the whole stack locally</span> with{" "}
              <code className="tabular rounded bg-secondary px-1 py-0.5 text-xs">make ledger</code> and{" "}
              <code className="tabular rounded bg-secondary px-1 py-0.5 text-xs">make up-all</code>, then open
              localhost:3000 (
              <a className="text-primary underline-offset-4 hover:underline" href={`${REPOSITORY}#quick-start`}>
                quick start
              </a>
              ).
            </p>
            <Button asChild variant="outline" className="w-full">
              <a href={`${REPOSITORY}#readme`}>
                <BookOpen />
                Read the README
              </a>
            </Button>
          </CardContent>
        </Card>
      </div>
    </main>
  );
}
