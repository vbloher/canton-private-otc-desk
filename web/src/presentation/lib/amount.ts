export function sanitizeDecimal(typed: string): string {
  const s = typed.replace(/[^0-9.]/g, "");
  const dot = s.indexOf(".");
  const whole = (dot === -1 ? s : s.slice(0, dot)).slice(0, 28);
  if (dot === -1) return whole;
  const fraction = s.slice(dot + 1).replace(/\./g, "").slice(0, 10);
  return `${whole === "" ? "0" : whole}.${fraction}`;
}
