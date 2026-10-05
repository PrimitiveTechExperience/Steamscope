import { Directive, input } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { BaseChartDirective } from 'ng2-charts';

import { PriceChartComponent } from './price-chart';
import { PricePoint } from '../../models/game';
import { textOf } from '../../../testing/factories';

// Chart.js needs a real canvas, which jsdom doesn't provide. The stub records
// what the component would have drawn.
@Directive({ selector: '[baseChart]' })
class FakeChartDirective {
  data = input<{ labels?: unknown[]; datasets: { data: number[] }[] }>();
  options = input<unknown>();
  type = input<string>();
}

function daysAgo(n: number): string {
  return new Date(Date.now() - n * 24 * 60 * 60 * 1000).toISOString();
}

function point(daysBack: number, price: number): PricePoint {
  return { date: daysAgo(daysBack), price, original_price: price, discount_percentage: 0 };
}

function render(inputs: { points: PricePoint[]; loading?: boolean; emptyMessage?: string; title?: string }) {
  TestBed.configureTestingModule({ imports: [PriceChartComponent] });
  TestBed.overrideComponent(PriceChartComponent, {
    remove: { imports: [BaseChartDirective] },
    add: { imports: [FakeChartDirective] },
  });
  const fixture = TestBed.createComponent(PriceChartComponent);
  for (const [key, value] of Object.entries(inputs)) fixture.componentRef.setInput(key, value);
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement };
}

const buttons = (el: HTMLElement) => Array.from(el.querySelectorAll('button'));

describe('PriceChartComponent', () => {
  it('shows a skeleton while loading, never the chart or empty message', () => {
    const { el } = render({ points: [point(5, 10), point(1, 8)], loading: true });
    expect(el.querySelector('.skeleton')).not.toBeNull();
    expect(el.querySelector('canvas')).toBeNull();
    expect(textOf(el)).not.toContain('Not enough');
  });

  it('draws the chart when there are at least two points in range', () => {
    const { el } = render({ points: [point(10, 12.5), point(5, 9.99), point(1, 7.5)] });
    expect(el.querySelector('canvas')).not.toBeNull();
    expect(el.querySelector('.skeleton')).toBeNull();
    expect(textOf(el)).not.toContain('Not enough');
  });

  it('shows the empty message when there are no points', () => {
    expect(textOf(render({ points: [] }).el)).toContain('Not enough price history for this range yet.');
  });

  it('shows the empty message for a single point (a line needs two)', () => {
    const { el } = render({ points: [point(1, 5)] });
    expect(textOf(el)).toContain('Not enough price history');
    expect(el.querySelector('canvas')).toBeNull();
  });

  it('uses a custom empty message and title', () => {
    const { el } = render({ points: [], emptyMessage: 'History starts at discovery.', title: 'Bundle Price History' });
    expect(textOf(el)).toContain('History starts at discovery.');
    expect(textOf(el)).toContain('Bundle Price History');
  });

  it('offers all six range tabs with 1 Year selected by default', () => {
    const { el } = render({ points: [] });
    expect(buttons(el).map((b) => textOf(b))).toEqual(['Week', 'Month', '3 Months', '6 Months', '1 Year', '2 Years']);
    const active = buttons(el).filter((b) => b.style.color === 'rgb(255, 255, 255)' || b.style.color === '#fff');
    expect(active.map((b) => textOf(b))).toEqual(['1 Year']);
  });

  it('labels each point with its calendar day, not the local day of the viewer', () => {
    const midnight = new Date();
    midnight.setUTCDate(midnight.getUTCDate() - 10);
    midnight.setUTCHours(0, 0, 0, 0);
    const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    const expected = `${months[midnight.getUTCMonth()]} ${midnight.getUTCDate()}, ${midnight.getUTCFullYear()}`;

    const { fixture } = render({ points: [{ date: midnight.toISOString(), price: 5, original_price: 5, discount_percentage: 0 }, point(1, 6)] });
    const chart = fixture.debugElement.query((d) => d.name === 'canvas').injector.get(FakeChartDirective);
    expect(chart.data()?.labels?.[0]).toBe(expected);
  });

  it('filters points by the selected range', () => {
    const { fixture, el } = render({ points: [point(400, 20), point(200, 15), point(20, 10), point(2, 8)] });
    const labels = () => (fixture.debugElement.query((d) => d.name === 'canvas').injector.get(FakeChartDirective).data()?.datasets[0].data ?? []);

    expect(labels()).toEqual([15, 10, 8]); // default 1 year drops the 400-day-old point

    buttons(el).find((b) => textOf(b) === 'Month')!.click();
    fixture.detectChanges();
    expect(labels()).toEqual([10, 8]);

    buttons(el).find((b) => textOf(b) === '2 Years')!.click();
    fixture.detectChanges();
    expect(labels()).toEqual([20, 15, 10, 8]);
  });

  it('falls back to the empty message when the chosen range has under two points', () => {
    const { fixture, el } = render({ points: [point(200, 15), point(150, 12)] });
    expect(el.querySelector('canvas')).not.toBeNull();
    buttons(el).find((b) => textOf(b) === 'Week')!.click();
    fixture.detectChanges();
    expect(el.querySelector('canvas')).toBeNull();
    expect(textOf(el)).toContain('Not enough price history');
  });
});
