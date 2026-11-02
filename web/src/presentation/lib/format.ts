export function formatAmount(value: string, decimals = 10): string {
  const negative = value.startsWith("-");
  const body = negative ? value.slice(1) : value;
  const [whole = "0", fraction = ""] = body.split(".");
  let frac = fraction.slice(0, Math.max(0, decimals)).replace(/0+$/, "");
  if (frac === "" && /^0*$/.test(whole) && /[1-9]/.test(fraction)) {
    frac = fraction.slice(0, 10).replace(/0+$/, "");
  }
  const grouped = whole.replace(/^0+(?=\d)/, "").replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return `${negative ? "-" : ""}${grouped}${frac === "" ? "" : `.${frac}`}`;
}

const PRICE_SIGNIFICANT_DIGITS = 6;
const PRICE_MIN_DECIMALS = 2;
const PRICE_MAX_DECIMALS = 10;

export function formatPrice(value: string): string {
  const negative = value.startsWith("-");
  const [whole = "0", fraction = ""] = (negative ? value.slice(1) : value).split(".");
  const wholeDigits = whole.replace(/^0+/, "").length;
  const zeros = wholeDigits === 0 ? fraction.length - fraction.replace(/^0+/, "").length : 0;
  const decimals = Math.min(
    PRICE_MAX_DECIMALS,
    Math.max(
      PRICE_MIN_DECIMALS,
      wholeDigits === 0 ? zeros + PRICE_SIGNIFICANT_DIGITS : PRICE_SIGNIFICANT_DIGITS - wholeDigits,
    ),
  );
  const units =
    BigInt(whole + fraction.slice(0, decimals).padEnd(decimals, "0")) + (fraction.charAt(decimals) >= "5" ? 1n : 0n);
  const digits = units.toString().padStart(decimals + 1, "0");
  return formatAmount(`${negative ? "-" : ""}${digits.slice(0, -decimals)}.${digits.slice(-decimals)}`, decimals);
}

export function formatCountdown(ms: number): string {
  const s = Math.ceil(Math.max(0, ms) / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${String(s % 60).padStart(2, "0")}s`;
  return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, "0")}m`;
}

export function formatTtl(seconds: number): string {
  if (seconds < 60) return `${seconds} s`;
  if (seconds % 3600 === 0) return `${seconds / 3600} h`;
  return `${Math.round(seconds / 60)} min`;
}

export function shortId(id: string): string {
  return id.length > 16 ? `${id.slice(0, 8)}…${id.slice(-6)}` : id;
}

export function formatTime(ms: number): string {
  return new Date(ms).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

export function formatDateTime(ms: number): string {
  return new Date(ms).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

export function count(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}
