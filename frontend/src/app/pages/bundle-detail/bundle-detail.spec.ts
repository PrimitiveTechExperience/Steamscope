import { Component, input } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { of, throwError } from 'rxjs';
import { HttpErrorResponse } from '@angular/common/http';

import { BundleDetailComponent } from './bundle-detail';
import { PriceChartComponent } from '../../components/price-chart/price-chart';
import { GamesService } from '../../services/games';
import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { fakeAuth, makeBundle, makeUser, textOf } from '../../../testing/factories';
import { BundleDetail } from '../../models/bundle';

@Component({ selector: 'app-price-chart', template: '<p class="chart-stub">{{ points().length }} points / {{ emptyMessage() }}</p>' })
class StubPriceChart {
  points = input<unknown[]>([]);
  emptyMessage = input('');
}

async function render(opts: { bundle?: BundleDetail | null; loggedIn?: boolean; submitGame?: ReturnType<typeof vi.fn> } = {}) {
  const getBundle = vi.fn().mockReturnValue(
    opts.bundle === null ? throwError(() => new HttpErrorResponse({ status: 404 })) : of(opts.bundle ?? makeBundle())
  );
  const submitGame = opts.submitGame ?? vi.fn().mockReturnValue(of({ kind: 'app', id: 2, status: 'awaiting_approval' }));

  TestBed.configureTestingModule({
    providers: [
      provideRouter([{ path: 'bundles/:bundle_id', component: BundleDetailComponent }]),
      { provide: GamesService, useValue: { getBundle } },
      { provide: AuthService, useValue: fakeAuth(opts.loggedIn === false ? null : makeUser()) },
      { provide: AccountService, useValue: { submitGame } },
    ],
  });
  TestBed.overrideComponent(BundleDetailComponent, {
    remove: { imports: [PriceChartComponent] },
    add: { imports: [StubPriceChart] },
  });
  const harness = await RouterTestingHarness.create();
  await harness.navigateByUrl('/bundles/5001', BundleDetailComponent);
  harness.detectChanges();
  return { harness, el: harness.routeNativeElement as HTMLElement, getBundle, submitGame };
}

const rows = (el: HTMLElement) => Array.from(el.querySelectorAll('ul li'));

describe('BundleDetailComponent', () => {
  it('loads the bundle from the route id and shows its details', async () => {
    const { el, getBundle } = await render();
    expect(getBundle).toHaveBeenCalledWith(5001);
    const text = textOf(el);
    expect(text).toContain('Starter Pack');
    expect(text).toContain('3 games');
    expect(text).toContain('-50%');
    expect(text).toContain('$40.00');
    expect(text).toContain('$20.00');
    const steam = Array.from(el.querySelectorAll('a')).find((a) => textOf(a) === 'View on Steam')!;
    expect(steam.getAttribute('href')).toBe('https://store.steampowered.com/bundle/5001');
    expect(steam.getAttribute('rel')).toContain('noopener');
  });

  it('explains that bundle history starts at discovery', async () => {
    const { el } = await render();
    expect(textOf(el.querySelector('.chart-stub'))).toContain('recorded daily from when we first found this bundle');
  });

  it('links games on the site and offers to request the rest', async () => {
    const { el } = await render();
    const [onSite, missing, requested] = rows(el);

    expect(onSite.querySelector('a')?.getAttribute('href')).toBe('/games/1');
    expect(onSite.querySelector('button')).toBeNull();

    expect(textOf(missing)).toContain('Not On The Site');
    expect(missing.querySelector('a')).toBeNull();
    expect(textOf(missing.querySelector('button'))).toBe('Request tracking');

    expect(textOf(requested)).toContain('Awaiting approval');
    expect(requested.querySelector('button')).toBeNull();
  });

  it('asks logged-out visitors to log in instead of showing the request button', async () => {
    const { el } = await render({ loggedIn: false });
    const missing = rows(el)[1];
    expect(missing.querySelector('button')).toBeNull();
    const link = missing.querySelector('a')!;
    expect(textOf(link)).toBe('Log in to request');
    expect(link.getAttribute('href')).toContain('/login');
    expect(decodeURIComponent(link.getAttribute('href')!)).toContain('returnUrl=/bundles/5001');
  });

  it('submits the game page URL when requested and then shows its status', async () => {
    const { harness, el, submitGame } = await render();
    (rows(el)[1].querySelector('button') as HTMLButtonElement).click();
    harness.detectChanges();

    expect(submitGame).toHaveBeenCalledWith('https://store.steampowered.com/app/2');
    expect(rows(el)[1].querySelector('button')).toBeNull();
    expect(textOf(rows(el)[1])).toContain('Awaiting approval');
  });

  it('shows the server error when a request fails', async () => {
    const submitGame = vi.fn().mockReturnValue(
      throwError(() => new HttpErrorResponse({ status: 429, error: { error: 'you can submit up to 5 games or bundles an hour' } }))
    );
    const { harness, el } = await render({ submitGame });
    (rows(el)[1].querySelector('button') as HTMLButtonElement).click();
    harness.detectChanges();

    expect(textOf(el)).toContain('You can submit up to 5 games or bundles an hour'); // capitalized
    expect(rows(el)[1].querySelector('button')).not.toBeNull(); // can try again
  });

  it('handles a bundle with no games', async () => {
    const { el } = await render({ bundle: makeBundle({ games: null, game_count: 0 }) });
    expect(rows(el)).toHaveLength(0);
    expect(textOf(el)).toContain('Included games');
  });

  it('shows a not-found message when the bundle does not exist', async () => {
    const { el } = await render({ bundle: null });
    expect(textOf(el)).toContain("That bundle couldn't be found.");
    expect(el.querySelector('h1')).toBeNull();
  });

  it('hides the discount badge for a full-price bundle', async () => {
    const { el } = await render({ bundle: makeBundle({ discount_percentage: 0, original_price: 20 }) });
    expect(textOf(el)).not.toContain('%');
    expect(el.querySelector('.line-through')).toBeNull();
  });
});
