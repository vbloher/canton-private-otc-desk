/**
 * Ledger money (Daml Numeric 10): exact bigint fixed point with ten fraction
 * digits, mirroring the backend's pkg/decimal. Products floor to ten digits,
 * the rounding Daml applies, so every preview equals what settles. Money never
 * passes through a float.
 */

/** Ten fraction digits: 1.0 on the ledger is 10^10 units. */
export const SCALE = 10;
const ONE = 10n ** BigInt(SCALE);
const BPS = 10_000n;

/** The wire format: up to 28 integer digits, up to 10 fraction digits, no sign. */
const DECIMAL = /^(\d{1,28})(?:\.(\d{1,10}))?$/;

export function isDecimal(text: string): boolean {
  return DECIMAL.test(text);
}

export function isPositiveDecimal(text: string): boolean {
  return isDecimal(text) && parseDecimal(text) > 0n;
}

/** "100000.50" → 1000005000000000n. Throws on anything off the wire format. */
export function parseDecimal(text: string): bigint {
  const m = DECIMAL.exec(text);
  if (m === null) throw new Error(`not a decimal: ${JSON.stringify(text)}`);
  const whole = m[1] ?? "0";
  const frac = (m[2] ?? "").padEnd(SCALE, "0");
  return BigInt(whole) * ONE + BigInt(frac);
}

/** Scaled units → canonical text: trailing zeros trimmed, one fraction digit kept ("5.0"). */
export function formatDecimal(units: bigint): string {
  const sign = units < 0n ? "-" : "";
  const abs = units < 0n ? -units : units;
  const frac = (abs % ONE).toString().padStart(SCALE, "0").replace(/0+$/, "");
  return `${sign}${(abs / ONE).toString()}.${frac === "" ? "0" : frac}`;
}

/** a × b floored to ten fraction digits (qty × price = notional). */
export function mulDecimal(a: string, b: string): string {
  return formatDecimal((parseDecimal(a) * parseDecimal(b)) / ONE);
}

/** amount × bps / 10000 floored (the desk fee on a notional). */
export function mulBps(amount: string, bps: number): string {
  return formatDecimal((parseDecimal(amount) * BigInt(bps)) / BPS);
}

export function addDecimal(a: string, b: string): string {
  return formatDecimal(parseDecimal(a) + parseDecimal(b));
}

export function subDecimal(a: string, b: string): string {
  return formatDecimal(parseDecimal(a) - parseDecimal(b));
}

export function cmpDecimal(a: string, b: string): -1 | 0 | 1 {
  const x = parseDecimal(a);
  const y = parseDecimal(b);
  return x < y ? -1 : x > y ? 1 : 0;
}
