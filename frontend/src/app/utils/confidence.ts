/**
 * How confident a forecast or a call is, as a word. The two middle bands are
 * kept apart on purpose: a "Low-medium" and a "High-medium" confidence are
 * different situations and should not read the same.
 */
export type ConfidenceBand = 'Very low' | 'Low' | 'Low-medium' | 'High-medium' | 'High' | 'Very high';

/** Lower bound of each band, from the top down. */
const BANDS: [number, ConfidenceBand][] = [
  [0.8, 'Very high'],
  [0.6, 'High'],
  [0.45, 'High-medium'],
  [0.3, 'Low-medium'],
  [0.15, 'Low'],
  [0, 'Very low'],
];

export function confidenceBand(confidence: number): ConfidenceBand {
  const c = Number.isFinite(confidence) ? Math.min(1, Math.max(0, confidence)) : 0;
  return BANDS.find(([min]) => c >= min)![1];
}

/** A 0-1 confidence as a percentage to the hundredth, e.g. 0.4375 -> "43.75%". */
export function confidencePercent(confidence: number): string {
  const c = Number.isFinite(confidence) ? Math.min(1, Math.max(0, confidence)) : 0;
  return `${(c * 100).toFixed(2)}%`;
}

/** "High-medium (54.37%)". */
export function confidenceText(confidence: number): string {
  return `${confidenceBand(confidence)} (${confidencePercent(confidence)})`;
}
