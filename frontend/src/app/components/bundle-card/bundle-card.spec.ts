import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { BundleCardComponent } from './bundle-card';
import { asListedBundle, makeBundle, textOf } from '../../../testing/factories';

function render(over = {}) {
  TestBed.configureTestingModule({ imports: [BundleCardComponent], providers: [provideRouter([])] });
  const fixture = TestBed.createComponent(BundleCardComponent);
  fixture.componentRef.setInput('bundle', asListedBundle(makeBundle(over)));
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('BundleCardComponent', () => {
  it('shows name, game count, price, original price and discount', () => {
    const text = textOf(render());
    expect(text).toContain('Starter Pack');
    expect(text).toContain('3 games');
    expect(text).toContain('$20.00');
    expect(text).toContain('$40.00');
    expect(text).toContain('-50%');
  });

  it('flags a bundle that is at its lowest recorded price', () => {
    const el = render({ at_record_low: true });
    expect(textOf(el.querySelector('.record-low-badge'))).toBe('Record low');
  });

  it('shows no record-low badge otherwise', () => {
    expect(render({ at_record_low: false }).querySelector('.record-low-badge')).toBeNull();
  });

  it('says which games matched a search when it was found through them', () => {
    const el = render({ matched_games: ['Portal', 'Portal 2'] });
    expect(textOf(el.querySelector('.matched-games'))).toBe('Contains Portal, Portal 2');
  });

  it('says nothing of the sort when it was not', () => {
    expect(render().querySelector('.matched-games')).toBeNull();
  });

  it('says nothing for an empty list of matches', () => {
    expect(render({ matched_games: [] }).querySelector('.matched-games')).toBeNull();
  });

  it('links to the bundle page', () => {
    expect(render().querySelector('a')?.getAttribute('href')).toBe('/bundles/5001');
  });

  it('omits the discount badge and strike-through price when not discounted', () => {
    const el = render({ discount_percentage: 0, original_price: 20 });
    expect(textOf(el)).not.toContain('%');
    expect(el.querySelector('.line-through')).toBeNull();
  });
});
