import { Check, Copy } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/presentation/components/ui/button";

/** Copies `value`; the icon ticks for a moment. */
export function CopyButton({
  value,
  label,
  size = "icon",
}: {
  value: string;
  label: string;
  size?: "icon" | "icon-xs";
}) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => setCopied(false), 1_500);
    return () => clearTimeout(t);
  }, [copied]);

  return (
    <Button
      type="button"
      variant={size === "icon" ? "outline" : "ghost"}
      size={size}
      aria-label={label}
      title={label}
      onClick={() => {
        navigator.clipboard.writeText(value).then(
          () => setCopied(true),
          () => toast.error("The browser did not allow copying — select the text instead."),
        );
      }}
    >
      {copied ? <Check /> : <Copy />}
    </Button>
  );
}
