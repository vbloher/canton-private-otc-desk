import { cn } from "@/presentation/lib/utils";

const HUE: Record<string, string> = {
  CBTC: "#f7931a",
  USDCX: "#2775ca",
  CC: "#b8860b",
};

function hueOf(symbol: string): string {
  let h = 0;
  for (const c of symbol) h = (h * 31 + c.charCodeAt(0)) % 360;
  return `hsl(${h} 45% 40%)`;
}

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
