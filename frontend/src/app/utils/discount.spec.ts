import { discountOf } from './discount';

const g = (price: number, original_price: number, discount_percentage: number) => ({ price, original_price, discount_percentage });

describe('discountOf', () => {
  it('uses the stored percentage', () => {
    expect(discountOf(g(5, 10, 50))).toBe(50);
  });

  it('rounds a fractional percentage, as the database returns numerics', () => {
    expect(discountOf(g(5, 10, 49.6))).toBe(50);
  });

  it('works the percentage out from the prices when none is stored', () => {
    expect(discountOf(g(14.99, 59.99, 0))).toBe(75);
    expect(discountOf(g(7.5, 10, Number.NaN))).toBe(25);
  });

  it('is 0 for a game at its regular price or with no price information', () => {
    expect(discountOf(g(10, 10, 0))).toBe(0);
    expect(discountOf(g(0, 0, 0))).toBe(0);
    expect(discountOf(g(10, 0, 0))).toBe(0);
  });

  it('is 0 when the original price is below the price (bad data)', () => {
    expect(discountOf(g(10, 5, 0))).toBe(0);
  });

  it('never exceeds 100', () => {
    expect(discountOf(g(0, 10, 250))).toBe(100);
    expect(discountOf(g(0, 10, 0))).toBe(100);
  });
});
