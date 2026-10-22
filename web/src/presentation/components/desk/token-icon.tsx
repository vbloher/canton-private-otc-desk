import { cn } from "@/presentation/lib/utils";

/** Known test instruments get their usual hue; anything else a stable one from its symbol. */
const HUE: Record<string, string> = {
  CBTC: "#f7931a",
  USDCX: "#2775ca",
  CETH: "#627eea",
};

function hueOf(symbol: string): string {
  let h = 0;
  for (const c of symbol) h = (h * 31 + c.charCodeAt(0)) % 360;
  return `hsl(${h} 45% 40%)`;
}

/** A monogram disc for a token. */
export function TokenIcon({ symbol, className }: { symbol: string; className?: string }) {
  const key = symbol.toUpperCase();
  const monogram = key.replace(/[^A-Z0-9]/g, "").slice(0, 2) || "?";
  return (
    <span
      aria-hidden
      className={cn(
        "flex size-8 flex-none items-center justify-center rounded-full text-[0.6875rem] font-bold text-white",
        className,
      )}
      style={{ backgroundColor: HUE[key] ?? hueOf(key) }}
    >
      {monogram}
    </span>
  );
}
