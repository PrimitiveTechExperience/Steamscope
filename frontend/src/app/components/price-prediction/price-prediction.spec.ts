import { Directive, input } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { HttpErrorResponse } from '@angular/common/http';
import { BaseChartDirective } from 'ng2-charts';
import { of, throwError, NEVER } from 'rxjs';

import { PricePredictionComponent } from './price-prediction';
import { GamesService } from '../../services/games';
import { Advice, Forecast } from '../../models/prediction';
import { textOf } from '../../../testing/factories';

// Chart.js needs a real canvas, which jsdom lacks; the stub records what would be drawn.
@Directive({ selector: '[baseChart]' })
class FakeChartDirective {
  data = input<{ labels?: unknown[]; datasets: { label?: string; data: number[] }[] }>();
  options = input<unknown>();
  type = input<string>();
}

function makeForecast(over: Partial<Forecast> = {}): Forecast {
  return {
    app_id: 730,
    generated_at: '2026-10-05T00:00:00Z',
    model: 'weibull_renewal',
    current_price: 20,
    regular_price: 20,
    current_discount_percent: 0,
    on_sale: false,
    history_days: 730,
    historic_low: 10,
    at_record_low: false,
    used_extended_history: false,
    typical_sale: { count: 12, median_depth_percent: 50, median_price: 10, median_duration_days: 10, median_interval_days: 60 },
    next_sale: { p25_days: 15, median_days: 21, p75_days: 28 },
    score: 96,
    confidence: 1,
    curve: [
      { date: '2026-10-06', days_ahead: 1, expected_price: 19.9, p_on_sale: 0.01, p_lower_by: 0.01 },
      { date: '2027-03-14', days_ahead: 160, expected_price: 15.5, p_on_sale: 0.2, p_lower_by: 0.98 },
      { date: '2028-10-04', days_ahead: 730, expected_price: 16, p_on_sale: 0.2, p_lower_by: 1 },
    ],
    horizons: [
      { days: 30, p_lower: 0.84, expected_price: 17, expected_low: 12.4 },
      { days: 90, p_lower: 0.99, expected_price: 16, expected_low: 10.9 },
      { days: 180, p_lower: 1, expected_price: 16, expected_low: 10.1 },
      { days: 365, p_lower: 1, expected_price: 16, expected_low: 10 },
      { days: 730, p_lower: 1, expected_price: 16, expected_low: 10 },
    ],
    cached: false,
    ...over,
  };
}

function makeAdvice(over: Partial<Advice> = {}): Advice {
  return {
    verdict: 'wait',
    score: 12,
    confidence: 0.8,
    reasons: [
      { code: 'full_price', text: 'Not on sale; its usual sale is 50% off', impact: -15 },
      { code: 'better_price_likely', text: '99% chance of a lower price within 90 days (about 45% cheaper on average)', impact: -37 },
      { code: 'at_historic_low', text: 'Within 2% of the lowest price on record ($10.00)', impact: 30 },
    ],
    patience_days: 90,
    chance_of_lower: 0.99,
    expected_saving_percent: 45,
    wait_until: '2026-10-26',
    at_record_low: false,
    personalized: false,
    generated_at: '2026-10-05T00:00:00Z',
    cached: false,
    ...over,
  };
}

interface Setup {
  forecast?: Forecast | 'loading' | HttpErrorResponse;
  advice?: Advice | 'loading' | HttpErrorResponse;
}

function render(setup: Setup = {}) {
  const pick = <T>(v: T | 'loading' | HttpErrorResponse | undefined, fallback: T) =>
    v === 'loading' ? NEVER : v instanceof HttpErrorResponse ? throwError(() => v) : of(v ?? fallback);
  const service = {
    getPrediction: vi.fn().mockReturnValue(pick(setup.forecast, makeForecast())),
    getAdvice: vi.fn().mockReturnValue(pick(setup.advice, makeAdvice())),
  };
  TestBed.configureTestingModule({ imports: [PricePredictionComponent], providers: [{ provide: GamesService, useValue: service }] });
  TestBed.overrideComponent(PricePredictionComponent, { remove: { imports: [BaseChartDirective] }, add: { imports: [FakeChartDirective] } });
  const fixture = TestBed.createComponent(PricePredictionComponent);
  fixture.componentRef.setInput('appId', 730);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const q = (selector: string) => el.querySelector(selector);
  return { fixture, el, service, q };
}

describe('PricePredictionComponent', () => {
  describe('loading the data', () => {
    it('requests the forecast and the advice for the game', () => {
      const { service } = render();
      expect(service.getPrediction).toHaveBeenCalledWith(730);
      expect(service.getAdvice).toHaveBeenCalledWith(730);
    });

    it('shows a skeleton while loading, and neither verdict nor chart', () => {
      const { el, q } = render({ forecast: 'loading', advice: 'loading' });
      expect(q('.skeleton')).not.toBeNull();
      expect(q('.verdict')).toBeNull();
      expect(q('canvas')).toBeNull();
      expect(textOf(el)).toContain('Buy now or wait?');
    });

    it('refetches when the refresh key changes (for example after watching the game)', () => {
      const { fixture, service } = render();
      expect(service.getAdvice).toHaveBeenCalledTimes(1);
      fixture.componentRef.setInput('refreshKey', { watched: true });
      fixture.detectChanges();
      expect(service.getAdvice).toHaveBeenCalledTimes(2);
    });

    it('says the forecast is unavailable when the request fails', () => {
      const err = new HttpErrorResponse({ status: 500 });
      const { el, q } = render({ forecast: err, advice: err });
      expect(textOf(el)).toContain("The price forecast isn't available right now");
      expect(q('.verdict')).toBeNull();
      expect(q('.skeleton')).toBeNull();
    });
  });

  describe('the verdict', () => {
    it.each([
      ['buy_now', 'Buy now', 'var(--color-discount)'],
      ['wait', 'Wait', 'var(--color-accent)'],
      ['toss_up', 'Toss-up', 'var(--color-surface-alt)'],
    ] as const)('shows %s as "%s"', (verdict, label, background) => {
      const { q } = render({ advice: makeAdvice({ verdict }) });
      const badge = q('.verdict') as HTMLElement;
      expect(textOf(badge)).toBe(label);
      expect(badge.style.background).toContain(background);
    });

    it('shows the score and how confident the call is, as a word and an exact percentage', () => {
      const { q } = render({ advice: makeAdvice({ score: 12, confidence: 0.8125 }) });
      expect(textOf(q('.advice-confidence'))).toBe('Score 12/100 · Confidence: Very high (81.25%)');
    });

    it.each([
      [0.9, 'Very high (90.00%)'],
      [0.6, 'High (60.00%)'],
      [0.5437, 'High-medium (54.37%)'],
      [0.39, 'Low-medium (39.00%)'],
      [0.2, 'Low (20.00%)'],
      [0.0123, 'Very low (1.23%)'],
    ])('shows a confidence of %s as "%s"', (confidence, text) => {
      expect(textOf(render({ advice: makeAdvice({ confidence }) }).q('.advice-confidence'))).toContain(`Confidence: ${text}`);
    });

    it('tells a good medium from a bad medium: a 90/100 call and a 21/100 call no longer read the same', () => {
      const strong = render({ advice: makeAdvice({ verdict: 'buy_now', score: 90, confidence: 0.54 }) });
      const strongText = textOf(strong.q('.advice-confidence'));
      TestBed.resetTestingModule();
      const weak = render({ advice: makeAdvice({ verdict: 'wait', score: 21, confidence: 0.39 }) });
      const weakText = textOf(weak.q('.advice-confidence'));

      expect(strongText).toContain('High-medium');
      expect(weakText).toContain('Low-medium');
    });

    it('names when to expect a sale only for a "wait" verdict', () => {
      const wait = render({ advice: makeAdvice({ verdict: 'wait', wait_until: '2026-10-26' }) });
      expect(textOf(wait.q('.wait-until'))).toContain('A sale is likely around');
      expect(textOf(wait.q('.wait-until strong'))).toBe('Oct 26, 2026');
      TestBed.resetTestingModule();
      const buy = render({ advice: makeAdvice({ verdict: 'buy_now', wait_until: null }) });
      expect(buy.q('.wait-until')).toBeNull();
    });

    it('shows the expected saving from waiting, when there is one', () => {
      expect(textOf(render({ advice: makeAdvice({ patience_days: 45, expected_saving_percent: 29.4 }) }).el)).toContain(
        'Waiting up to 45 days saves about 29% on average.'
      );
      TestBed.resetTestingModule();
      expect(textOf(render({ advice: makeAdvice({ expected_saving_percent: 0 }) }).el)).not.toContain('saves about');
    });
  });

  describe('the record-low note', () => {
    const atLow = makeForecast({ at_record_low: true, current_price: 10, current_discount_percent: 50, on_sale: true, historic_low: 10, history_days: 2190 });

    it('tells the user straight away when the price is the lowest on record', () => {
      const { q } = render({ forecast: atLow, advice: makeAdvice({ verdict: 'buy_now', at_record_low: true }) });
      const note = textOf(q('.record-low'));
      expect(note).toContain('Record low');
      expect(note).toContain('This is the lowest price on record ($10.00, from 2190 days of history)');
    });

    it('puts the note above the verdict, where it is seen first', () => {
      const { q } = render({ forecast: atLow });
      const note = q('.record-low')!;
      const verdict = q('.verdict')!;
      expect(note.compareDocumentPosition(verdict) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it('is absent when the price is not a record low', () => {
      expect(render().q('.record-low')).toBeNull();
    });

    it('is also shown when there is too little history for a forecast', () => {
      const thin = makeForecast({ model: 'insufficient', at_record_low: true, history_days: 40, curve: [], horizons: [] });
      expect(textOf(render({ forecast: thin, advice: makeAdvice({ verdict: 'not_enough_data', reasons: [] }) }).q('.record-low'))).toContain('lowest price on record');
    });

    it('is absent for a free game', () => {
      const free = makeForecast({ model: 'free', at_record_low: false, current_price: 0, historic_low: 0, curve: [], horizons: [] });
      expect(render({ forecast: free, advice: makeAdvice({ verdict: 'free', reasons: [] }) }).q('.record-low')).toBeNull();
    });

    function emitted(forecast: Forecast): boolean[] {
      const seen: boolean[] = [];
      TestBed.configureTestingModule({
        imports: [PricePredictionComponent],
        providers: [{ provide: GamesService, useValue: { getPrediction: () => of(forecast), getAdvice: () => of(makeAdvice()) } }],
      });
      TestBed.overrideComponent(PricePredictionComponent, { remove: { imports: [BaseChartDirective] }, add: { imports: [FakeChartDirective] } });
      const fixture = TestBed.createComponent(PricePredictionComponent);
      fixture.componentRef.setInput('appId', 730);
      fixture.componentInstance.recordLowChange.subscribe((v) => seen.push(v));
      fixture.detectChanges();
      TestBed.tick();
      return seen;
    }

    it('tells the page about it, so the header can show a badge', () => {
      expect(emitted(atLow).at(-1)).toBe(true);
    });

    it('tells the page when it is not a record low', () => {
      expect(emitted(makeForecast()).at(-1)).toBe(false);
    });
  });

  describe('the reasons', () => {
    it('lists every reason with its signed impact, coloured by direction', () => {
      const { q } = render();
      const rows = Array.from(q('ul.reasons')!.querySelectorAll('li'));
      expect(rows.map((r) => textOf(r))).toEqual([
        '-15 Not on sale; its usual sale is 50% off',
        '-37 99% chance of a lower price within 90 days (about 45% cheaper on average)',
        '+30 Within 2% of the lowest price on record ($10.00)',
      ]);
      const colors = rows.map((r) => (r.querySelector('.impact') as HTMLElement).style.color);
      expect(colors).toEqual(['var(--color-accent)', 'var(--color-accent)', 'var(--color-discount)']);
    });

    it('omits the list when there are no reasons', () => {
      expect(render({ advice: makeAdvice({ reasons: [] }) }).q('ul.reasons')).toBeNull();
    });

    it('explains that anonymous or non-watching users get a general call', () => {
      expect(textOf(render({ advice: makeAdvice({ personalized: false }) }).q('.generic-note'))).toContain('This is a general call');
      TestBed.resetTestingModule();
      expect(render({ advice: makeAdvice({ personalized: true }) }).q('.generic-note')).toBeNull();
    });
  });

  describe('the forecast details', () => {
    it('describes the next sale with its likely range', () => {
      expect(textOf(render().q('.next-sale'))).toBe('Next sale expected in about 21 days (likely 15 to 28 days)');
    });

    it('uses the singular for one day, and drops the range when it is unknown', () => {
      const f = makeForecast({ next_sale: { p25_days: null, median_days: 1, p75_days: null } });
      expect(textOf(render({ forecast: f }).q('.next-sale'))).toBe('Next sale expected in about 1 day');
    });

    it('says when no sale is expected', () => {
      const f = makeForecast({ next_sale: { p25_days: null, median_days: null, p75_days: null } });
      expect(textOf(render({ forecast: f }).q('.next-sale'))).toBe('No sale is expected within two years');
    });

    it('describes the usual sale and the lowest price on record', () => {
      const { el, q } = render();
      expect(textOf(q('.typical-sale'))).toBe('12 past sales, usually 50% off for about 10 days');
      expect(textOf(el)).toContain('Typically $10.00');
      expect(textOf(q('.historic-low'))).toBe('$10.00');
      expect(textOf(el)).toContain('730 days of history');
    });

    it('handles a game with no past sales', () => {
      const f = makeForecast({
        model: 'no_sales_seen',
        confidence: 0.15,
        typical_sale: { count: 0, median_depth_percent: 0, median_price: 0, median_duration_days: 7, median_interval_days: null },
      });
      const { el, q } = render({ forecast: f });
      expect(textOf(q('.typical-sale'))).toBe('No past sales seen');
      expect(textOf(el)).not.toContain('Typically');
      expect(textOf(el)).toContain('Forecast confidence: Low (15.00%)');
    });

    it('shows the forecast confidence with its exact percentage', () => {
      expect(textOf(render({ forecast: makeForecast({ confidence: 0.4375 }) }).q('.forecast-confidence'))).toBe(
        'Forecast confidence: Low-medium (43.75%)'
      );
      TestBed.resetTestingModule();
      expect(textOf(render().q('.forecast-confidence'))).toBe('Forecast confidence: Very high (100.00%)');
    });

    it('shows one row per horizon with the chance of a lower price and the expected low', () => {
      const rows = Array.from(render().q('table.horizons')!.querySelectorAll('tbody tr')).map((r) => textOf(r));
      expect(rows).toEqual(['1 month 84% $12.40', '3 months 99% $10.90', '6 months 100% $10.10', '1 year 100% $10.00', '2 years 100% $10.00']);
    });

    it('always includes the disclaimer', () => {
      expect(textOf(render().q('.disclaimer'))).toContain('not a guarantee');
    });
  });

  describe('the chart', () => {
    function chartOf(fixture: ReturnType<typeof render>['fixture']) {
      return fixture.debugElement.query((d) => d.name === 'canvas').injector.get(FakeChartDirective);
    }

    it('plots expected price and the chance of a lower price over the whole curve', () => {
      const { fixture, q } = render();
      expect(q('canvas')).not.toBeNull();
      const data = chartOf(fixture).data()!;
      expect(data.datasets.map((d) => d.label)).toEqual(['Expected price', 'Chance of a lower price (%)']);
      expect(data.datasets[0].data).toEqual([19.9, 15.5, 16]);
      expect(data.datasets[1].data).toEqual([1, 98, 100]);
    });

    it('labels the axis by month, in UTC so no label is a day early', () => {
      const { fixture } = render();
      expect(chartOf(fixture).data()!.labels).toEqual(["Oct '26", "Mar '27", "Oct '28"]);
    });
  });

  describe('a free game', () => {
    const free = makeForecast({ model: 'free', current_price: 0, regular_price: 0, historic_low: 0, curve: [], horizons: [], score: 0 });
    const advice = makeAdvice({ verdict: 'free', score: 100, confidence: 1, reasons: [{ code: 'free', text: 'This game is free, so there is nothing to wait for', impact: 0 }], expected_saving_percent: 0, wait_until: null });

    it('says there is nothing to wait for instead of showing a forecast', () => {
      const { el, q } = render({ forecast: free, advice });
      expect(textOf(q('.verdict'))).toBe('Free');
      expect(textOf(q('.no-forecast'))).toContain('free to play');
      expect(textOf(el)).toContain('nothing to wait for');
      expect(q('table.horizons')).toBeNull();
      expect(q('canvas')).toBeNull();
    });

    it('shows no score, confidence, saving or "general call" note', () => {
      const { el, q } = render({ forecast: free, advice });
      expect(q('.advice-confidence')).toBeNull();
      expect(q('.forecast-confidence')).toBeNull();
      expect(q('.generic-note')).toBeNull();
      expect(textOf(el)).not.toContain('saves about');
    });
  });

  describe('when there is too little history', () => {
    const thin = makeForecast({ model: 'insufficient', history_days: 31, curve: [], horizons: [] });

    it('explains how much history there is instead of a forecast', () => {
      const { el, q } = render({ forecast: thin, advice: makeAdvice({ verdict: 'not_enough_data', reasons: [], expected_saving_percent: 0 }) });
      expect(textOf(q('.no-forecast'))).toContain('We need about two months of price history');
      expect(textOf(q('.no-forecast'))).toContain('31 days so far');
      expect(q('table.horizons')).toBeNull();
      expect(q('canvas')).toBeNull();
      expect(textOf(q('.verdict'))).toBe('Not enough data');
      expect(textOf(el)).not.toContain('Score');
      expect(textOf(el)).not.toContain('Forecast confidence');
    });

    it('uses the singular for one day of history', () => {
      expect(textOf(render({ forecast: makeForecast({ ...thin, history_days: 1 }) }).q('.no-forecast'))).toContain('1 day so far');
    });

    it('still honours a target price the user set', () => {
      const advice = makeAdvice({
        verdict: 'buy_now', score: 90, personalized: true, wait_until: null, expected_saving_percent: 0,
        reasons: [{ code: 'target_met', text: 'The price ($9.00) is at or below your target of $10.00', impact: 40 }],
      });
      const { el, q } = render({ forecast: thin, advice });
      expect(textOf(q('.verdict'))).toBe('Buy now');
      expect(textOf(el)).toContain('at or below your target');
    });
  });
});
