/**
 * Display formatting. Money stays a string end to end: digits are cut, never
 * rounded through a float.
 */

/**
 * "50000.1234" with 2 digits → "50,000.12": thousands grouped, cut to the
 * instrument's display digits, trailing zeros dropped. An amount that is not
 * zero never reads as zero; it keeps its digits instead.
 */
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

/** Time left: "45s", "2m 05s", "1h 02m". */
export function formatCountdown(ms: number): string {
  const s = Math.ceil(Math.max(0, ms) / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${String(s % 60).padStart(2, "0")}s`;
  return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, "0")}m`;
}

/** A lifetime choice: "30 s", "5 min", "1 h". */
export function formatTtl(seconds: number): string {
  if (seconds < 60) return `${seconds} s`;
  if (seconds % 3600 === 0) return `${seconds / 3600} h`;
  return `${Math.round(seconds / 60)} min`;
}

/** "1220abcd…abcdef": enough of a long id to recognise; show the full value on hover. */
export function shortId(id: string): string {
  return id.length > 16 ? `${id.slice(0, 8)}…${id.slice(-6)}` : id;
}

export function formatTime(ms: number): string {
  return new Date(ms).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

export function formatDateTime(ms: number): string {
  return new Date(ms).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

/** "1 quote", "3 quotes". */
export function count(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}
