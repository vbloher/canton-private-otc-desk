import { RadioGroupItem } from "@/presentation/components/ui/radio-group";
import { TokenIcon } from "./token-icon";

export function TokenChip({ symbol, disabled }: { symbol: string; disabled?: boolean }) {
  return (
    <RadioGroupItem
      value={symbol}
      disabled={disabled}
      className="flex items-center gap-1.5 rounded-full border border-transparent bg-card/50 py-1 pr-2.5 pl-1 text-sm text-muted-foreground transition-colors hover:bg-card hover:text-foreground data-checked:border-primary data-checked:bg-card data-checked:font-medium data-checked:text-foreground"
    >
      <TokenIcon symbol={symbol} className="size-5 text-[0.5625rem]" />
      {symbol}
    </RadioGroupItem>
  );
}
