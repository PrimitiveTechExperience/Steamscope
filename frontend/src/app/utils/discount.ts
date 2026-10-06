/** The fields a discount is read from. */
export interface Priced {
  price: number;
  original_price: number;
  discount_percentage: number;
}

/**
 * How many percent off a game is right now, as a whole number (0 when it isn't
 * discounted). Uses the stored percentage and, when that is missing, works it
 * out from the prices, so a game that is visibly cheaper than its regular price
 * never shows up without its discount badge.
 */
export function discountOf(game: Priced): number {
  const stored = Math.round(Number(game.discount_percentage) || 0);
  if (stored > 0) return Math.min(stored, 100);
  if (game.original_price > 0 && game.price >= 0 && game.original_price > game.price) {
    return Math.round((1 - game.price / game.original_price) * 100);
  }
  return 0;
}
