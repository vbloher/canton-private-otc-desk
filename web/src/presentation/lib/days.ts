/** Consecutive runs of a newest-first feed per local calendar day: "Today", "Yesterday", then dates. */
export interface DayGroup<T> {
  key: string;
  label: string;
  items: T[];
}

function dayKey(ms: number): string {
  const d = new Date(ms);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

export function groupByDay<T extends { at: number }>(items: readonly T[], now: number): DayGroup<T>[] {
  const today = dayKey(now);
  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  const yesterdayKey = dayKey(yesterday.getTime());
  const thisYear = new Date(now).getFullYear();

  const label = (ms: number, key: string): string => {
    if (key === today) return "Today";
    if (key === yesterdayKey) return "Yesterday";
    const d = new Date(ms);
    return d.toLocaleDateString(undefined, {
      weekday: "short",
      month: "short",
      day: "numeric",
      ...(d.getFullYear() === thisYear ? {} : { year: "numeric" }),
    });
  };

  const groups: DayGroup<T>[] = [];
  for (const item of items) {
    const key = dayKey(item.at);
    const last = groups.at(-1);
    if (last?.key === key) last.items.push(item);
    else groups.push({ key, label: label(item.at, key), items: [item] });
  }
  return groups;
}
