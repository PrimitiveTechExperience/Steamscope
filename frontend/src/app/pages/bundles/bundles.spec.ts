import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { NEVER, of, throwError } from 'rxjs';
import { HttpErrorResponse } from '@angular/common/http';

import { BundlesComponent } from './bundles';
import { GamesService } from '../../services/games';
import { asListedBundle, makeBundle, textOf } from '../../../testing/factories';

const bundle = (id: number, name: string, over = {}) => asListedBundle(makeBundle({ bundle_id: id, name, ...over }));

function render(getBundles = vi.fn().mockReturnValue(of([bundle(1, 'Valve Complete Pack'), bundle(2, 'Indie Pack')]))) {
  TestBed.configureTestingModule({
    imports: [BundlesComponent],
    providers: [provideRouter([]), { provide: GamesService, useValue: { getBundles } }],
  });
  const fixture = TestBed.createComponent(BundlesComponent);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const box = () => el.querySelector<HTMLInputElement>('.bundle-search')!;
  const type = (value: string) => {
    box().value = value;
    box().dispatchEvent(new Event('input'));
    fixture.detectChanges();
  };
  const press = () => {
    box().dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter' }));
    fixture.detectChanges();
  };
  const click = () => {
    (el.querySelector('.search-button') as HTMLButtonElement).click();
    fixture.detectChanges();
  };
  const names = () => Array.from(el.querySelectorAll('app-bundle-card h2')).map((h) => textOf(h));
  return { fixture, el, getBundles, box, type, press, click, names };
}

describe('BundlesComponent', () => {
  it('lists every bundle to begin with', () => {
    const { getBundles, names } = render();
    expect(getBundles).toHaveBeenCalledWith(undefined, '');
    expect(names()).toEqual(['Valve Complete Pack', 'Indie Pack']);
  });

  it('has a search box with a button, like the games page', () => {
    const { box, el } = render();
    expect(box().placeholder).toBe('Search bundles or the games in them');
    expect(box().getAttribute('aria-label')).toBe('Search bundles');
    expect(box().getAttribute('maxlength')).toBe('100');
    expect(textOf(el.querySelector('.search-button'))).toBe('Search');
  });

  it('searches when Enter is pressed', () => {
    const { type, press, getBundles } = render();
    type('valve');
    expect(getBundles).toHaveBeenCalledTimes(1); // typing alone does not search
    press();
    expect(getBundles).toHaveBeenLastCalledWith(undefined, 'valve');
  });

  it('searches when the button is clicked, trimming what was typed', () => {
    const { type, click, getBundles } = render();
    type('  portal  ');
    click();
    expect(getBundles).toHaveBeenLastCalledWith(undefined, 'portal');
  });

  it('shows what the search found, including which game matched', () => {
    const getBundles = vi.fn().mockReturnValueOnce(of([bundle(1, 'A'), bundle(2, 'B')]))
      .mockReturnValueOnce(of([bundle(3, 'Puzzle Starter', { matched_games: ['Portal 2'] })]));
    const { type, press, names, el } = render(getBundles);
    type('portal');
    press();
    expect(names()).toEqual(['Puzzle Starter']);
    expect(textOf(el.querySelector('.matched-games'))).toBe('Contains Portal 2');
  });

  it('lists everything again when the box is cleared and searched', () => {
    const { type, press, getBundles } = render();
    type('valve');
    press();
    type('   ');
    press();
    expect(getBundles).toHaveBeenLastCalledWith(undefined, '');
  });

  it('says when a search matches nothing, naming it', () => {
    const getBundles = vi.fn().mockReturnValueOnce(of([bundle(1, 'A')])).mockReturnValueOnce(of([]));
    const { type, press, el } = render(getBundles);
    type('zzzz');
    press();
    expect(textOf(el.querySelector('.no-bundles'))).toBe('No bundles match "zzzz".');
  });

  it('says none are tracked when there are none at all', () => {
    const { el } = render(vi.fn().mockReturnValue(of([])));
    expect(textOf(el.querySelector('.no-bundles'))).toBe('No bundles tracked yet.');
  });

  it('shows a skeleton while searching', () => {
    const getBundles = vi.fn().mockReturnValueOnce(of([bundle(1, 'A')])).mockReturnValueOnce(NEVER);
    const { type, press, el } = render(getBundles);
    type('slow');
    press();
    expect(el.querySelector('.skeleton')).not.toBeNull();
    expect(el.querySelectorAll('app-bundle-card')).toHaveLength(0);
  });

  it('treats a failed search as no results rather than breaking', () => {
    const getBundles = vi.fn().mockReturnValueOnce(of([bundle(1, 'A')])).mockReturnValueOnce(throwError(() => new HttpErrorResponse({ status: 500 })));
    const { type, press, el } = render(getBundles);
    type('boom');
    press();
    expect(textOf(el.querySelector('.no-bundles'))).toBe('No bundles match "boom".');
    // And the next search still works.
    getBundles.mockReturnValue(of([bundle(4, 'Recovered')]));
    type('again');
    press();
    expect(textOf(el.querySelector('app-bundle-card h2'))).toBe('Recovered');
  });

  it('keeps what was typed in the box', () => {
    const { type, press, box } = render();
    type('valve pack');
    press();
    expect(box().value).toBe('valve pack');
  });
});
