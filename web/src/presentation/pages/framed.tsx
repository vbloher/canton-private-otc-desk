/** What a Pages build shows inside another page instead of the desk: the auto-signer must never run framed. */
export function FramedNotice() {
  return (
    <main className="p-6 text-sm text-muted-foreground">
      Private OTC Desk does not run inside other pages. Open it directly in its own tab.
    </main>
  );
}
