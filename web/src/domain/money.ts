export const SCALE = 10;
const ONE = 10n ** BigInt(SCALE);
const BPS = 10_000n;

const DECIMAL = /^(\d{1,28})(?:\.(\d{1,10}))?$/;

export function isDecimal(text: string): boolean {
  return DECIMAL.test(text);
}

export function isPositiveDecimal(text: string): boolean {
  return isDecimal(text) && parseDecimal(text) > 0n;
}

export function parseDecimal(text: string): bigint {
  const m = DECIMAL.exec(text);
  if (m === null) throw new Error(`not a decimal: ${JSON.stringify(text)}`);
  const whole = m[1] ?? "0";
  const frac = (m[2] ?? "").padEnd(SCALE, "0");
  return BigInt(whole) * ONE + BigInt(frac);
}

export function formatDecimal(units: bigint): string {
  const sign = units < 0n ? "-" : "";
  const abs = units < 0n ? -units : units;
  const frac = (abs % ONE).toString().padStart(SCALE, "0").replace(/0+$/, "");
  return `${sign}${(abs / ONE).toString()}.${frac === "" ? "0" : frac}`;
}

export function mulDecimal(a: string, b: string): string {
  return formatDecimal((parseDecimal(a) * parseDecimal(b)) / ONE);
}

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
