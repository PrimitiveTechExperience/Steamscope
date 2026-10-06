import { TestBed } from '@angular/core/testing';

import { BundleValueComponent } from './bundle-value';
import { BundleValue } from '../../models/bundle';
import { makeBundleValue, textOf } from '../../../testing/factories';

function render(value: BundleValue = makeBundleValue(), historyDays = 90) {
  TestBed.configureTestingModule({ imports: [BundleValueComponent] });
  const fixture = TestBed.createComponent(BundleValueComponent);
  fixture.componentRef.setInput('value', value);
  fixture.componentRef.setInput('historyDays', historyDays);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  return { fixture, el, q: (s: string) => el.querySelector(s) };
}

describe('BundleValueComponent', () => {
  describe('the verdict', () => {
    it.each([
      ['great_deal', 'Great deal', 'var(--color-discount)'],
      ['good_deal', 'Good deal', 'var(--color-discount)'],
      ['fair', 'Fair', 'var(--color-surface-alt)'],
      ['poor_value', 'Poor value', 'var(--color-accent)'],
    ] as const)('shows %s as "%s"', (verdict, label, background) => {
      const { q } = render(makeBundleValue({ verdict }));
      const badge = q('.verdict') as HTMLElement;
      expect(textOf(badge)).toBe(label);
      expect(badge.style.background).toContain(background);
    });

    it('shows the score', () => {
      expect(textOf(render(makeBundleValue({ score: 73 })).q('.score'))).toBe('Score 73/100');
    });
  });

  describe('the price comparison', () => {
    it('compares the bundle with the regular prices and with buying separately today', () => {
      const { q } = render();
      expect(textOf(q('.regular'))).toBe('$60.00');
      expect(textOf(q('.separate'))).toBe('$36.00');
      expect(textOf(q('.bundle'))).toBe('$20.00');
      expect(textOf(q('.vs-regular'))).toBe('Saves $40.00 (66.7%)');
      expect(textOf(q('.vs-separate'))).toBe('Saves $16.00 (44.4%)');
    });

    it('says plainly when the bundle costs more than the games bought separately', () => {
      const dearer = makeBundleValue({
        verdict: 'poor_value',
        totals: { regular: 60, separate: 30, bundle: 40, priced_items: 2, items: 2 },
        savings_vs_separate: -10,
        savings_vs_separate_percent: -33.33,
        savings_vs_regular: 20,
        savings_vs_regular_percent: 33.33,
      });
      const { q } = render(dearer);
      expect(textOf(q('.vs-separate'))).toBe('Costs $10.00 more (33.3%)');
      expect((q('.vs-separate') as HTMLElement).style.color).toContain('var(--color-accent)');
      // The bundle still looks cheaper against regular prices - which is the trap.
      expect(textOf(q('.vs-regular'))).toBe('Saves $20.00 (33.3%)');
      expect((q('.vs-regular') as HTMLElement).style.color).toContain('var(--color-discount)');
    });

    it('says "No difference" when the prices match', () => {
      const same = makeBundleValue({ savings_vs_separate: 0, savings_vs_separate_percent: 0 });
      expect(textOf(render(same).q('.vs-separate'))).toBe('No difference');
    });
  });

  describe('the reasons', () => {
    it('lists each reason with its signed impact, coloured by direction', () => {
      const { q } = render();
      const rows = Array.from(q('ul.reasons')!.querySelectorAll('li'));
      expect(rows.map((r) => textOf(r))).toEqual([
        '+30 Buying these games separately today costs $36.00; the bundle is $20.00, saving $16.00 (44%)',
        "+15 67% below the games' regular prices ($60.00)",
        '-10 25% above the lowest price recorded for this bundle ($16.00)',
      ]);
      const colors = rows.map((r) => (r.querySelector('.impact') as HTMLElement).style.color);
      expect(colors).toEqual(['var(--color-discount)', 'var(--color-discount)', 'var(--color-accent)']);
    });

    it('omits the list when there are none', () => {
      expect(render(makeBundleValue({ reasons: [] })).q('ul.reasons')).toBeNull();
    });
  });

  describe('the record-low note', () => {
    const low = makeBundleValue({ at_record_low: true, record_low: 20 });

    it('tells the user when the bundle is at the lowest price recorded for it', () => {
      const { q } = render(low, 90);
      const note = textOf(q('.record-low'));
      expect(note).toContain('Record low');
      expect(note).toContain('The lowest price recorded for this bundle ($20.00, from 90 days of history)');
    });

    it('uses the singular for one day', () => {
      expect(textOf(render(low, 1).q('.record-low'))).toContain('from 1 day of history');
    });

    it('sits above the verdict', () => {
      const { q } = render(low);
      expect(q('.record-low')!.compareDocumentPosition(q('.verdict')!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it('is absent when it is not a record low', () => {
      expect(render().q('.record-low')).toBeNull();
    });
  });

  describe('the per-game table', () => {
    it('shows each game with its price today, regular price and share of the bundle', () => {
      const rows = Array.from(render().q('table.games')!.querySelectorAll('tbody tr')).map((r) => textOf(r));
      expect(rows[0]).toBe('Game 1 $12.00 -40% $20.00 $6.67');
      expect(rows).toHaveLength(3);
    });

    it('marks games that are cheaper on their own, and explains the mark', () => {
      const value = makeBundleValue({
        cheaper_alone_count: 1,
        items: [
          { app_id: 1, name: 'Pricey', price: 30, regular_price: 30, discount_percent: 0, bundle_share: 15, cheaper_alone: false, priced: true },
          { app_id: 2, name: 'Bargain', price: 4, regular_price: 10, discount_percent: 60, bundle_share: 5, cheaper_alone: true, priced: true },
        ],
      });
      const { q, el } = render(value);
      const rows = Array.from(q('table.games')!.querySelectorAll('tbody tr'));
      expect(rows[0].querySelector('.cheaper-alone')).toBeNull();
      expect(textOf(rows[1].querySelector('.cheaper-alone'))).toBe('Cheaper alone');
      expect(textOf(q('.cheaper-note'))).toContain('costs less on its own today than its share');
      expect(textOf(el)).toContain('Bargain');
    });

    it('omits the explanation when no game is cheaper alone', () => {
      expect(render().q('.cheaper-note')).toBeNull();
    });

    it('shows "price unknown" for a game without a price, and says the picture is partial', () => {
      const value = makeBundleValue({
        completeness: 0.6667,
        totals: { regular: 40, separate: 24, bundle: 20, priced_items: 2, items: 3 },
        items: [
          { app_id: 1, name: 'A', price: 12, regular_price: 20, discount_percent: 40, bundle_share: 10, cheaper_alone: false, priced: true },
          { app_id: 2, name: 'B', price: 12, regular_price: 20, discount_percent: 40, bundle_share: 10, cheaper_alone: false, priced: true },
          { app_id: 3, name: 'Mystery', price: 0, regular_price: 0, discount_percent: 0, bundle_share: 0, cheaper_alone: false, priced: false },
        ],
      });
      const { q } = render(value);
      expect(textOf(q('td.unknown'))).toBe('price unknown');
      expect(textOf(q('.incomplete'))).toBe("Prices for 1 of 3 games aren't available, so this only covers part of the bundle.");
    });

    it('does not mention missing prices when all are known', () => {
      expect(render().q('.incomplete')).toBeNull();
    });
  });

  describe('when it cannot be judged', () => {
    const unjudged = makeBundleValue({ verdict: 'not_enough_data', score: 50, reasons: [], items: [], totals: { regular: 0, separate: 0, bundle: 20, priced_items: 0, items: 2 } });

    it('says so instead of showing figures', () => {
      const { el, q } = render(unjudged);
      expect(textOf(q('.verdict'))).toBe('Not enough data');
      expect(textOf(q('.no-data'))).toContain("isn't enough price information");
      expect(q('table.comparison')).toBeNull();
      expect(q('table.games')).toBeNull();
      expect(q('.score')).toBeNull();
      expect(textOf(el)).not.toContain('Score');
    });

    it('still shows a record low it knows about', () => {
      const { q } = render({ ...unjudged, at_record_low: true, record_low: 20 });
      expect(textOf(q('.record-low'))).toContain('Record low');
    });
  });
});
