import { confidenceBand, confidencePercent, confidenceText } from './confidence';

describe('confidence', () => {
  it.each([
    [0, 'Very low'],
    [0.1499, 'Very low'],
    [0.15, 'Low'],
    [0.2999, 'Low'],
    [0.3, 'Low-medium'],
    [0.4499, 'Low-medium'],
    [0.45, 'High-medium'],
    [0.5999, 'High-medium'],
    [0.6, 'High'],
    [0.7999, 'High'],
    [0.8, 'Very high'],
    [1, 'Very high'],
  ])('puts %s in the "%s" band', (value, band) => {
    expect(confidenceBand(value)).toBe(band);
  });

  it('separates a good medium from a bad medium', () => {
    // Real examples: a 90/100 call and a 21/100 call used to both read "Medium".
    expect(confidenceBand(0.54)).toBe('High-medium');
    expect(confidenceBand(0.39)).toBe('Low-medium');
    expect(confidenceBand(0.54)).not.toBe(confidenceBand(0.39));
  });

  it('treats out-of-range and invalid values safely', () => {
    expect(confidenceBand(-1)).toBe('Very low');
    expect(confidenceBand(NaN)).toBe('Very low');
    expect(confidenceBand(5)).toBe('Very high');
  });

  it('formats a percentage to the hundredth', () => {
    expect(confidencePercent(0.4375)).toBe('43.75%');
    expect(confidencePercent(0.36)).toBe('36.00%');
    expect(confidencePercent(1)).toBe('100.00%');
    expect(confidencePercent(0)).toBe('0.00%');
    expect(confidencePercent(0.63938)).toBe('63.94%');
  });

  it('clamps the percentage to 0-100%', () => {
    expect(confidencePercent(1.5)).toBe('100.00%');
    expect(confidencePercent(-0.2)).toBe('0.00%');
    expect(confidencePercent(NaN)).toBe('0.00%');
  });

  it('combines the word and the percentage', () => {
    expect(confidenceText(0.5437)).toBe('High-medium (54.37%)');
  });
});
