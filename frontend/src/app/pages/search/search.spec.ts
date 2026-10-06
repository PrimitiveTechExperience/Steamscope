import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of } from 'rxjs';

import { SearchComponent } from './search';
import { GamesService } from '../../services/games';
import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { fakeAuth, makeGame, makeUser, textOf } from '../../../testing/factories';

function render(opts: { query?: Record<string, string>; loggedIn?: boolean; recent?: unknown[] } = {}) {
  const getGames = vi.fn().mockReturnValue(of({ games: [makeGame()], total: 1, limit: 60, offset: 0 }));
  const addRecentSearch = vi.fn().mockReturnValue(of(undefined));
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: GamesService, useValue: { getGames, getFilterOptions: () => of({ genres: [], tags: [], developers: [], publishers: [], languages: [] }) } },
      { provide: AuthService, useValue: fakeAuth(opts.loggedIn ? makeUser() : null) },
      { provide: AccountService, useValue: { getRecentSearches: () => of(opts.recent ?? []), addRecentSearch } },
      { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap(opts.query ?? {}) } } },
    ],
  });
  const fixture = TestBed.createComponent(SearchComponent);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const steps = () => Array.from(el.querySelectorAll<HTMLButtonElement>('.discount-step'));
  const step = (label: string) => steps().find((b) => textOf(b) === label)!;
  const lastQuery = () => getGames.mock.calls.at(-1)![0];
  return { fixture, el, getGames, addRecentSearch, steps, step, lastQuery };
}

describe('SearchComponent discount filter', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  async function settle(fixture: { detectChanges(): void }) {
    await vi.advanceTimersByTimeAsync(200); // the search is debounced
    fixture.detectChanges();
  }

  it('offers any / on sale / 25% / 50% / 75% and starts on "Any"', async () => {
    const { fixture, steps, step, lastQuery } = render();
    await settle(fixture);
    expect(steps().map((b) => textOf(b))).toEqual(['Any', 'On sale', '25%+', '50%+', '75%+']);
    expect(step('Any').getAttribute('aria-pressed')).toBe('true');
    expect(lastQuery().minDiscount).toBeUndefined();
  });

  it('searches for games at least that discounted when a step is chosen', async () => {
    const { fixture, step, lastQuery } = render();
    await settle(fixture);
    step('50%+').click();
    await settle(fixture);
    expect(lastQuery().minDiscount).toBe(50);
    expect(step('50%+').getAttribute('aria-pressed')).toBe('true');
    expect(step('Any').getAttribute('aria-pressed')).toBe('false');
  });

  it('"On sale" means any discount at all', async () => {
    const { fixture, step, lastQuery } = render();
    await settle(fixture);
    step('On sale').click();
    await settle(fixture);
    expect(lastQuery().minDiscount).toBe(1);
  });

  it('goes back to every game when "Any" is chosen again', async () => {
    const { fixture, step, lastQuery } = render();
    await settle(fixture);
    step('75%+').click();
    await settle(fixture);
    step('Any').click();
    await settle(fixture);
    expect(lastQuery().minDiscount).toBeUndefined();
  });

  it('combines with the price filter', async () => {
    const { fixture, el, step, lastQuery } = render();
    await settle(fixture);
    const max = el.querySelectorAll<HTMLInputElement>('input[type="number"]')[1];
    max.value = '20';
    max.dispatchEvent(new Event('input'));
    step('25%+').click();
    await settle(fixture);
    expect(lastQuery()).toMatchObject({ maxPrice: 20, minDiscount: 25 });
  });

  it('opens already filtered from /search?minDiscount=50, ignoring junk values', async () => {
    const good = render({ query: { minDiscount: '50' } });
    await settle(good.fixture);
    expect(good.lastQuery().minDiscount).toBe(50);
    expect(good.step('50%+').getAttribute('aria-pressed')).toBe('true');

    TestBed.resetTestingModule();
    for (const bad of ['abc', '0', '101', '5.5', '-3']) {
      TestBed.resetTestingModule();
      const r = render({ query: { minDiscount: bad } });
      await settle(r.fixture);
      expect(r.lastQuery().minDiscount, bad).toBeUndefined();
    }
  });

  it('saves a discount-only search for logged-in users, and restores it from recent searches', async () => {
    const { fixture, el, step, addRecentSearch } = render({ loggedIn: true, recent: [{ min_discount: 50 }, { min_discount: 1 }] });
    await settle(fixture);
    step('25%+').click();
    await vi.advanceTimersByTimeAsync(2100);
    fixture.detectChanges();
    expect(addRecentSearch).toHaveBeenCalledWith(expect.objectContaining({ min_discount: 25 }));

    const recent = Array.from(el.querySelectorAll('button')).filter((b) => / off$|^On sale$/.test(textOf(b)) && !b.classList.contains('discount-step'));
    expect(recent.map((b) => textOf(b))).toEqual(['25%+ off', '50%+ off', 'On sale']);
    recent[1].click();
    await settle(fixture);
    expect(step('50%+').getAttribute('aria-pressed')).toBe('true');
  });

  it('does not save an unfiltered search', async () => {
    const { fixture, addRecentSearch } = render({ loggedIn: true });
    await vi.advanceTimersByTimeAsync(2500);
    fixture.detectChanges();
    expect(addRecentSearch).not.toHaveBeenCalled();
  });
});
