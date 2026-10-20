import { RecentSearch } from '../models/user';

/** Where a signed-out visitor's recent searches are kept in the browser. */
export const LOCAL_RECENT_KEY = 'steamscope.recentSearches';

const MAX_TEXT = 100;
const MAX_LIST = 30;

/**
 * Reads the recent searches this browser kept. What is stored can have been
 * edited by hand or written by an older version of the site, so anything that
 * is not a well-formed search is dropped rather than trusted. Storage that is
 * unavailable (private mode, blocked cookies) reads as empty.
 */
export function loadLocalRecentSearches(max: number): RecentSearch[] {
  let raw: string | null;
  try {
    raw = localStorage.getItem(LOCAL_RECENT_KEY);
  } catch {
    return [];
  }
  if (!raw) return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!Array.isArray(parsed)) return [];
  return parsed.map(tidyRecentSearch).filter((s): s is RecentSearch => s !== null).slice(0, max);
}

/** Keeps the list in this browser. Failing to (storage full or blocked) is not an error worth showing. */
export function saveLocalRecentSearches(list: RecentSearch[]): void {
  try {
    localStorage.setItem(LOCAL_RECENT_KEY, JSON.stringify(list));
  } catch {
    // Nothing to do: the list still works for this visit.
  }
}

/**
 * A search with only what was actually set (no empty lists or blanks), or null
 * when nothing was. Used both on what is read back and on what is stored.
 */
export function tidyRecentSearch(value: unknown): RecentSearch | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  const v = value as Record<string, unknown>;
  const out: RecentSearch = {};
  if (typeof v['search'] === 'string' && v['search'].trim()) out.search = v['search'].slice(0, MAX_TEXT);
  for (const key of ['genres', 'tags', 'languages', 'developers', 'publishers'] as const) {
    const list = v[key];
    if (Array.isArray(list)) {
      const strings = list.filter((x): x is string => typeof x === 'string' && x.trim() !== '').map((x) => x.slice(0, MAX_TEXT)).slice(0, MAX_LIST);
      if (strings.length) out[key] = strings;
    }
  }
  for (const key of ['min_price', 'max_price'] as const) {
    const n = v[key];
    if (typeof n === 'number' && Number.isFinite(n) && n >= 0) out[key] = n;
  }
  const d = v['min_discount'];
  if (typeof d === 'number' && Number.isInteger(d) && d >= 1 && d <= 100) out.min_discount = d;
  if (v['include_bundles'] === true) out.include_bundles = true;
  return Object.keys(out).length ? out : null;
}
