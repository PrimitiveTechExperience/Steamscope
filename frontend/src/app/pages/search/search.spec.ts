import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of } from 'rxjs';

import { MAX_CHIP_LABEL, SearchComponent, shortLabel } from './search';
import { LOCAL_RECENT_KEY } from '../../utils/local-recent-searches';

// Signed-out visitors keep recent searches in the browser; no test may see another test's.
beforeEach(() => localStorage.clear());
import { GamesService } from '../../services/games';
import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { asListedBundle, fakeAuth, makeBundle, makeGame, makeUser, textOf } from '../../../testing/factories';

function render(opts: { query?: Record<string, string>; loggedIn?: boolean; recent?: unknown[] } = {}) {
  const getGames = vi.fn().mockReturnValue(of({ games: [makeGame()], total: 1, limit: 60, offset: 0 }));
  const addRecentSearch = vi.fn().mockReturnValue(of(undefined));
  const getRecentSearches = vi.fn().mockReturnValue(of(opts.recent ?? []));
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: GamesService, useValue: { getGames, getFilterOptions: () => of({ genres: [], tags: [], developers: [], publishers: [], languages: [] }) } },
      { provide: AuthService, useValue: fakeAuth(opts.loggedIn ? makeUser() : null) },
      { provide: AccountService, useValue: { getRecentSearches, addRecentSearch } },
      { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap(opts.query ?? {}) } } },
    ],
  });
  const fixture = TestBed.createComponent(SearchComponent);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const steps = () => Array.from(el.querySelectorAll<HTMLButtonElement>('.discount-step'));
  const step = (label: string) => steps().find((b) => textOf(b) === label)!;
  const lastQuery = () => getGames.mock.calls.at(-1)![0];
  return { fixture, el, getGames, addRecentSearch, getRecentSearches, steps, step, lastQuery };
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

  it('still shows only discounted games when the server ignores the filter', async () => {
    const all = [
      makeGame({ app_id: 1, name: 'Full Price', price: 20, original_price: 20, discount_percentage: 0 }),
      makeGame({ app_id: 2, name: 'Mild Sale', price: 15, original_price: 20, discount_percentage: 25 }),
      makeGame({ app_id: 3, name: 'Big Sale', price: 5, original_price: 20, discount_percentage: 75 }),
      makeGame({ app_id: 4, name: 'Unlabelled Sale', price: 8, original_price: 20, discount_percentage: 0 }), // 60% off by its prices
    ];
    const { fixture, el, step, getGames } = render();
    getGames.mockReturnValue(of({ games: all, total: 4, limit: 60, offset: 0 }));
    await settle(fixture);
    const shown = () => Array.from(el.querySelectorAll('app-game-card h2')).map((h) => textOf(h));

    expect(shown()).toEqual(['Full Price', 'Mild Sale', 'Big Sale', 'Unlabelled Sale']);
    step('50%+').click();
    await settle(fixture);
    expect(shown()).toEqual(['Big Sale', 'Unlabelled Sale']);
    step('On sale').click();
    await settle(fixture);
    expect(shown()).toEqual(['Mild Sale', 'Big Sale', 'Unlabelled Sale']);
    step('Any').click();
    await settle(fixture);
    expect(shown()).toHaveLength(4);
  });

  it('says so when nothing is discounted enough', async () => {
    const { fixture, el, step, getGames } = render();
    getGames.mockReturnValue(of({ games: [makeGame({ price: 20, original_price: 20, discount_percentage: 0 })], total: 1, limit: 60, offset: 0 }));
    await settle(fixture);
    step('75%+').click();
    await settle(fixture);
    expect(textOf(el)).toContain('No games match these filters.');
  });

  it('does not save an unfiltered search', async () => {
    const { fixture, addRecentSearch } = render({ loggedIn: true });
    await vi.advanceTimersByTimeAsync(2500);
    fixture.detectChanges();
    expect(addRecentSearch).not.toHaveBeenCalled();
  });
});

const bundleResult = (id: number, name: string, over = {}) => asListedBundle(makeBundle({ bundle_id: id, name, ...over }));

describe('SearchComponent text search and bundles', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  async function settle(fixture: { detectChanges(): void }, ms = 200) {
    await vi.advanceTimersByTimeAsync(ms);
    fixture.detectChanges();
  }
  const type = (el: HTMLElement, value: string) => {
    const box = el.querySelector<HTMLInputElement>('.search-box')!;
    box.value = value;
    box.dispatchEvent(new Event('input'));
  };
  const toggle = (el: HTMLElement) => el.querySelector<HTMLInputElement>('.include-bundles-box')!;
  const flip = (el: HTMLElement) => toggle(el).click();

  describe('the search box', () => {
    it('is there, empty, and sends nothing until something is typed', async () => {
      const { fixture, el, lastQuery } = render();
      await settle(fixture);
      expect(el.querySelector<HTMLInputElement>('.search-box')!.value).toBe('');
      expect(lastQuery().search).toBeUndefined();
    });

    it('searches for what is typed, trimmed, once typing pauses', async () => {
      const { fixture, el, getGames, lastQuery } = render();
      await settle(fixture);
      const before = getGames.mock.calls.length;
      type(el, '  half li');
      type(el, '  half life  ');
      await settle(fixture);
      expect(lastQuery().search).toBe('half life');
      expect(getGames.mock.calls.length - before).toBe(1); // one request, not one per keystroke
    });

    it('searches for everything again when cleared or left blank', async () => {
      const { fixture, el, lastQuery } = render();
      await settle(fixture);
      type(el, 'portal');
      await settle(fixture);
      type(el, '   ');
      await settle(fixture);
      expect(lastQuery().search).toBeUndefined();
    });

    it('combines with the other filters', async () => {
      const { fixture, el, step, lastQuery } = render();
      await settle(fixture);
      type(el, 'portal');
      step('50%+').click();
      await settle(fixture);
      expect(lastQuery()).toMatchObject({ search: 'portal', minDiscount: 50 });
    });

    it('opens with the text from /search?search=', async () => {
      const { fixture, el, lastQuery } = render({ query: { search: 'left 4 dead' } });
      await settle(fixture);
      expect(el.querySelector<HTMLInputElement>('.search-box')!.value).toBe('left 4 dead');
      expect(lastQuery().search).toBe('left 4 dead');
    });

    it('limits what can be typed', async () => {
      const { el } = render();
      expect(el.querySelector('.search-box')!.getAttribute('maxlength')).toBe('100');
      const { el: seeded } = (() => {
        TestBed.resetTestingModule();
        return render({ query: { search: 'x'.repeat(500) } });
      })();
      expect(seeded.querySelector<HTMLInputElement>('.search-box')!.value.length).toBe(100);
    });
  });

  describe('the bundles switch', () => {
    it('is off to begin with, and no bundles are requested', async () => {
      const { fixture, el, lastQuery } = render();
      await settle(fixture);
      expect(toggle(el).checked).toBe(false);
      expect(lastQuery().includeBundles).toBeUndefined();
      expect(el.querySelector('.bundle-results')).toBeNull();
    });

    it('asks for bundles when switched on, and stops asking when switched off', async () => {
      const { fixture, el, lastQuery } = render();
      await settle(fixture);
      flip(el);
      await settle(fixture);
      expect(lastQuery().includeBundles).toBe(true);
      flip(el);
      await settle(fixture);
      expect(lastQuery().includeBundles).toBeUndefined();
    });

    it('says what it does, and the search box says it covers bundles', async () => {
      const { fixture, el } = render();
      await settle(fixture);
      expect(textOf(el.querySelector('.bundles-filter'))).toContain('Also show bundles with a matching name, or with a matching game in them.');
      expect(el.querySelector<HTMLInputElement>('.search-box')!.placeholder).toBe('Search games by name or description');
      flip(el);
      fixture.detectChanges();
      expect(el.querySelector<HTMLInputElement>('.search-box')!.placeholder).toBe('Search games and bundles by name or description');
    });

    it('opens switched on from /search?bundles=1', async () => {
      const { fixture, el, lastQuery } = render({ query: { bundles: '1' } });
      await settle(fixture);
      expect(toggle(el).checked).toBe(true);
      expect(lastQuery().includeBundles).toBe(true);
    });

    it('ignores any other value of the bundles parameter', async () => {
      const { fixture, el } = render({ query: { bundles: 'maybe' } });
      await settle(fixture);
      expect(toggle(el).checked).toBe(false);
    });
  });

  describe('showing bundles', () => {
    const response = (bundles: unknown[], games = [makeGame()]) => of({ games, total: games.length, limit: 60, offset: 0, bundles });

    it('lists matching bundles after the games, with a count', async () => {
      const { fixture, el, getGames } = render({ query: { bundles: '1', search: 'valve' } });
      getGames.mockReturnValue(response([bundleResult(1, 'Valve Complete Pack'), bundleResult(2, 'Valve Starter')]));
      await settle(fixture);
      const section = el.querySelector('.bundle-results')!;
      expect(textOf(section.querySelector('h2'))).toBe('Bundles (2)');
      expect(Array.from(section.querySelectorAll('app-bundle-card h2')).map((h) => textOf(h))).toEqual(['Valve Complete Pack', 'Valve Starter']);
      expect(el.querySelectorAll('.games-results app-game-card')).toHaveLength(1);
      const order = el.querySelector('.games-results')!.compareDocumentPosition(section);
      expect(order & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy(); // bundles come after the games
    });

    it('says which game in a bundle matched', async () => {
      const { fixture, el, getGames } = render({ query: { bundles: '1', search: 'portal' } });
      getGames.mockReturnValue(response([bundleResult(1, 'Puzzle Starter', { matched_games: ['Portal', 'Portal 2'] }), bundleResult(2, 'Portal Collection')]));
      await settle(fixture);
      const cards = Array.from(el.querySelectorAll('app-bundle-card'));
      expect(textOf(cards[0].querySelector('.matched-games'))).toBe('Contains Portal, Portal 2');
      expect(cards[1].querySelector('.matched-games')).toBeNull(); // found by its own name
    });

    it('links each bundle to its page', async () => {
      const { fixture, el, getGames } = render({ query: { bundles: '1' } });
      getGames.mockReturnValue(response([bundleResult(5001, 'Linked Pack')]));
      await settle(fixture);
      expect(el.querySelector('app-bundle-card a')!.getAttribute('href')).toBe('/bundles/5001');
    });

    it('shows no bundle section when the switch is off, even if bundles came back', async () => {
      const { fixture, el, getGames } = render();
      getGames.mockReturnValue(response([bundleResult(1, 'Stray Bundle')]));
      await settle(fixture);
      expect(el.querySelector('.bundle-results')).toBeNull();
    });

    it('shows no bundle section when there are none', async () => {
      const { fixture, el, getGames } = render({ query: { bundles: '1' } });
      getGames.mockReturnValue(response([]));
      await settle(fixture);
      expect(el.querySelector('.bundle-results')).toBeNull();
    });

    it('shows bundles even when no game matched, without the "nothing found" message', async () => {
      const { fixture, el, getGames } = render({ query: { bundles: '1', search: 'pack' } });
      getGames.mockReturnValue(response([bundleResult(1, 'Mystery Pack')], []));
      await settle(fixture);
      expect(el.querySelectorAll('app-bundle-card')).toHaveLength(1);
      expect(el.querySelector('.no-results')).toBeNull();
    });

    it('does not show bundles while results are loading', async () => {
      const { fixture, el, getGames } = render({ query: { bundles: '1' } });
      getGames.mockReturnValue(response([bundleResult(1, 'Pack')]));
      await settle(fixture);
      expect(el.querySelector('.bundle-results')).not.toBeNull();
      type(el, 'x');
      await settle(fixture, 10); // typed, but the debounce has not fired
      expect(el.querySelector('.bundle-results')).not.toBeNull(); // still the old results, not a half state
    });

    it('does not break when the server sends no bundle list at all', async () => {
      const { fixture, el, getGames } = render({ query: { bundles: '1' } });
      getGames.mockReturnValue(of({ games: [makeGame()], total: 1, limit: 60, offset: 0 }));
      await settle(fixture);
      expect(el.querySelectorAll('app-game-card')).toHaveLength(1);
      expect(el.querySelector('.bundle-results')).toBeNull();
    });
  });

  describe('when nothing matches', () => {
    it('says no games match when bundles are off', async () => {
      const { fixture, el, getGames } = render();
      getGames.mockReturnValue(of({ games: [], total: 0, limit: 60, offset: 0 }));
      await settle(fixture);
      expect(textOf(el.querySelector('.no-results'))).toBe('No games match these filters.');
    });

    it('says no games or bundles match when bundles are on', async () => {
      const { fixture, el, getGames } = render({ query: { bundles: '1' } });
      getGames.mockReturnValue(of({ games: [], total: 0, limit: 60, offset: 0, bundles: [] }));
      await settle(fixture);
      expect(textOf(el.querySelector('.no-results'))).toBe('No games or bundles match these filters.');
    });

    it('copes with a null game list from an older server', async () => {
      const { fixture, el, getGames, step } = render();
      getGames.mockReturnValue(of({ games: null, total: 0, limit: 60, offset: 0 }));
      await settle(fixture);
      step('50%+').click(); // the browser-side discount filter runs over the list
      await settle(fixture);
      expect(textOf(el.querySelector('.no-results'))).toBe('No games match these filters.');
    });
  });

  describe('filters a bundle cannot satisfy', () => {
    it('explains why bundles are left out while a genre is chosen', async () => {
      const { fixture, el } = render({ query: { bundles: '1', genres: 'Action' } });
      await settle(fixture);
      expect(textOf(el.querySelector('.bundles-left-out'))).toContain('left out while one of those is set');
    });

    it.each(['genres', 'tags', 'developers', 'publishers', 'languages'])('does so for %s', async (key) => {
      const { fixture, el } = render({ query: { bundles: '1', [key]: 'Something' } });
      await settle(fixture);
      expect(el.querySelector('.bundles-left-out')).not.toBeNull();
    });

    it('says nothing when bundles are off, or nothing like that is chosen', async () => {
      const off = render({ query: { genres: 'Action' } });
      await settle(off.fixture);
      expect(off.el.querySelector('.bundles-left-out')).toBeNull();
      TestBed.resetTestingModule();
      const on = render({ query: { bundles: '1' } });
      await settle(on.fixture);
      expect(on.el.querySelector('.bundles-left-out')).toBeNull();
    });

    it('does not mention price or discount, which bundles do honour', async () => {
      const { fixture, el, step } = render({ query: { bundles: '1' } });
      step('50%+').click();
      await settle(fixture);
      expect(el.querySelector('.bundles-left-out')).toBeNull();
    });
  });

  describe('recent searches', () => {
    it('remembers the text and the bundles switch', async () => {
      const { fixture, el, addRecentSearch } = render({ loggedIn: true });
      await settle(fixture);
      type(el, 'portal');
      flip(el);
      await vi.advanceTimersByTimeAsync(2100);
      fixture.detectChanges();
      expect(addRecentSearch).toHaveBeenCalledWith(expect.objectContaining({ search: 'portal', include_bundles: true }));
    });

    it('saves a text-only search, but not the switch on its own', async () => {
      const a = render({ loggedIn: true });
      type(a.el, 'half life');
      await vi.advanceTimersByTimeAsync(2100);
      expect(a.addRecentSearch).toHaveBeenCalledTimes(1);

      TestBed.resetTestingModule();
      const b = render({ loggedIn: true });
      flip(b.el);
      await vi.advanceTimersByTimeAsync(2500);
      expect(b.addRecentSearch).not.toHaveBeenCalled();
    });

    it('labels and restores a remembered search', async () => {
      const { fixture, el } = render({ loggedIn: true, recent: [{ search: 'portal', include_bundles: true }, { search: 'half life' }] });
      await settle(fixture);
      const chips = Array.from(el.querySelectorAll('button')).filter((b) => /^"/.test(textOf(b)));
      expect(chips.map((c) => textOf(c))).toEqual(['"portal" · + bundles', '"half life"']);
      chips[0].click();
      await settle(fixture);
      expect(el.querySelector<HTMLInputElement>('.search-box')!.value).toBe('portal');
      expect(toggle(el).checked).toBe(true);
      chips[1].click();
      await settle(fixture);
      expect(el.querySelector<HTMLInputElement>('.search-box')!.value).toBe('half life');
      expect(toggle(el).checked).toBe(false); // restoring a search restores its switch too
    });
  });
});

describe('shortLabel', () => {
  it('leaves a short label alone', () => {
    expect(shortLabel('"portal" · + bundles')).toBe('"portal" · + bundles');
    expect(shortLabel('x'.repeat(MAX_CHIP_LABEL))).toBe('x'.repeat(MAX_CHIP_LABEL));
  });

  it('cuts a long one to the limit, ending in an ellipsis', () => {
    const cut = shortLabel('y'.repeat(200));
    expect(Array.from(cut)).toHaveLength(MAX_CHIP_LABEL);
    expect(cut.endsWith('…')).toBe(true);
  });

  it('does not leave a space before the ellipsis', () => {
    expect(shortLabel('word '.repeat(40))).not.toMatch(/ …$/);
  });

  it('counts characters, so emoji and accents are not split', () => {
    const cut = shortLabel('😀'.repeat(100));
    expect(Array.from(cut)).toHaveLength(MAX_CHIP_LABEL);
    expect(Array.from(cut).slice(0, -1).every((c) => c === '😀')).toBe(true);
  });

  it('can be given another limit', () => {
    expect(shortLabel('abcdefghij', 5)).toBe('abcd…');
  });
});

describe('SearchComponent recent search chips', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  const chips = (el: HTMLElement) => Array.from(el.querySelectorAll<HTMLButtonElement>('.recent-chip'));

  it('shows at most five, even if the server sends more', async () => {
    const recent = Array.from({ length: 8 }, (_, i) => ({ search: `search ${i + 1}` }));
    const { fixture, el } = render({ loggedIn: true, recent });
    await vi.advanceTimersByTimeAsync(200);
    fixture.detectChanges();
    expect(chips(el).map((c) => textOf(c))).toEqual(['"search 1"', '"search 2"', '"search 3"', '"search 4"', '"search 5"']);
  });

  it('keeps it at five as new searches are added, newest first', async () => {
    const recent = Array.from({ length: 5 }, (_, i) => ({ search: `old ${i + 1}` }));
    const { fixture, el } = render({ loggedIn: true, recent });
    const box = el.querySelector<HTMLInputElement>('.search-box')!;
    box.value = 'newest';
    box.dispatchEvent(new Event('input'));
    await vi.advanceTimersByTimeAsync(2100);
    fixture.detectChanges();
    const labels = chips(el).map((c) => textOf(c));
    expect(labels).toHaveLength(5);
    expect(labels[0]).toBe('"newest"');
    expect(labels).not.toContain('"old 5"'); // the oldest one dropped off
  });

  it('cuts a long search to 50 characters with an ellipsis, and shows it whole on hover', async () => {
    const long = 'a very long search '.repeat(10).trim();
    const { fixture, el } = render({ loggedIn: true, recent: [{ search: long, genres: ['Action', 'RPG', 'Strategy', 'Simulation'] }] });
    await vi.advanceTimersByTimeAsync(200);
    fixture.detectChanges();
    const chip = chips(el)[0];
    const shown = textOf(chip);
    expect(Array.from(shown).length).toBeLessThanOrEqual(50);
    expect(shown.endsWith('…')).toBe(true);
    expect(chip.title).toContain(long); // the full label is in the tooltip
    expect(chip.classList.contains('truncate')).toBe(true); // and the chip cannot widen the row
    expect(chip.classList.contains('max-w-full')).toBe(true);
  });

  it('leaves short labels whole', async () => {
    const { fixture, el } = render({ loggedIn: true, recent: [{ search: 'portal' }] });
    await vi.advanceTimersByTimeAsync(200);
    fixture.detectChanges();
    expect(textOf(chips(el)[0])).toBe('"portal"');
  });

  it('restores the whole search when a cut-down chip is clicked', async () => {
    const long = 'x'.repeat(95);
    const { fixture, el } = render({ loggedIn: true, recent: [{ search: long }] });
    await vi.advanceTimersByTimeAsync(200);
    fixture.detectChanges();
    chips(el)[0].click();
    await vi.advanceTimersByTimeAsync(200);
    fixture.detectChanges();
    expect(el.querySelector<HTMLInputElement>('.search-box')!.value).toBe(long);
  });
});

describe('the chip label limit', () => {
  it('is 50 characters', () => {
    expect(MAX_CHIP_LABEL).toBe(50);
    expect(shortLabel('z'.repeat(50))).toBe('z'.repeat(50)); // exactly 50 is left whole
    expect(shortLabel('z'.repeat(51))).toBe('z'.repeat(49) + '…');
  });
});

describe('SearchComponent recent searches for signed-out visitors', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  const chips = (el: HTMLElement) => Array.from(el.querySelectorAll<HTMLButtonElement>('.recent-chip')).map((c) => textOf(c));
  const stored = () => JSON.parse(localStorage.getItem(LOCAL_RECENT_KEY) ?? '[]');
  const type = (el: HTMLElement, value: string) => {
    const box = el.querySelector<HTMLInputElement>('.search-box')!;
    box.value = value;
    box.dispatchEvent(new Event('input'));
  };
  async function save(fixture: { detectChanges(): void }) {
    await vi.advanceTimersByTimeAsync(2100);
    fixture.detectChanges();
  }

  it('shows the searches kept in this browser', async () => {
    localStorage.setItem(LOCAL_RECENT_KEY, JSON.stringify([{ search: 'portal', include_bundles: true }, { min_discount: 75 }]));
    const { fixture, el } = render();
    await vi.advanceTimersByTimeAsync(200);
    fixture.detectChanges();
    expect(chips(el)).toEqual(['"portal" · + bundles', '75%+ off']);
    expect(textOf(el)).toContain('Recent:');
  });

  it('keeps a new search in the browser, not on the server', async () => {
    const { fixture, el, addRecentSearch, getRecentSearches } = render();
    type(el, 'half life');
    await save(fixture);
    expect(stored()).toEqual([{ search: 'half life' }]);
    expect(chips(el)).toEqual(['"half life"']);
    expect(addRecentSearch).not.toHaveBeenCalled();
    expect(getRecentSearches).not.toHaveBeenCalled();
  });

  it('keeps the newest five, newest first, without repeats', async () => {
    localStorage.setItem(LOCAL_RECENT_KEY, JSON.stringify([1, 2, 3, 4, 5].map((i) => ({ search: `old ${i}` }))));
    const { fixture, el } = render();
    type(el, 'old 3'); // repeating one moves it to the front
    await save(fixture);
    expect(stored().map((s: { search: string }) => s.search)).toEqual(['old 3', 'old 1', 'old 2', 'old 4', 'old 5']);
    type(el, 'brand new');
    await save(fixture);
    expect(stored().map((s: { search: string }) => s.search)).toEqual(['brand new', 'old 3', 'old 1', 'old 2', 'old 4']);
    expect(chips(el)).toHaveLength(5);
  });

  it('remembers filters and the bundles switch too', async () => {
    const { fixture, el, step } = render();
    step('50%+').click();
    el.querySelector<HTMLInputElement>('.include-bundles-box')!.click();
    type(el, 'pack');
    await save(fixture);
    expect(stored()).toEqual([{ search: 'pack', include_bundles: true, min_discount: 50 }]);
  });

  it('does not keep an empty search', async () => {
    const { fixture } = render();
    await save(fixture);
    expect(localStorage.getItem(LOCAL_RECENT_KEY)).toBeNull();
  });

  it('restores a kept search when its chip is clicked', async () => {
    localStorage.setItem(LOCAL_RECENT_KEY, JSON.stringify([{ search: 'portal', include_bundles: true }]));
    const { fixture, el } = render();
    await vi.advanceTimersByTimeAsync(200);
    fixture.detectChanges();
    el.querySelector<HTMLButtonElement>('.recent-chip')!.click();
    await vi.advanceTimersByTimeAsync(200);
    fixture.detectChanges();
    expect(el.querySelector<HTMLInputElement>('.search-box')!.value).toBe('portal');
    expect(el.querySelector<HTMLInputElement>('.include-bundles-box')!.checked).toBe(true);
  });

  it('ignores junk in storage instead of breaking the page', async () => {
    localStorage.setItem(LOCAL_RECENT_KEY, '{not json');
    const { fixture, el } = render();
    await vi.advanceTimersByTimeAsync(200);
    fixture.detectChanges();
    expect(chips(el)).toEqual([]);
    expect(el.querySelector('.search-box')).not.toBeNull();
  });

  it('still works when the browser will not store anything', async () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota exceeded');
    });
    const { fixture, el } = render();
    type(el, 'portal');
    await save(fixture);
    expect(chips(el)).toEqual(['"portal"']); // shown for this visit, even though it cannot be kept
  });

  it('uses the server, not the browser, once signed in', async () => {
    localStorage.setItem(LOCAL_RECENT_KEY, JSON.stringify([{ search: 'from this browser' }]));
    const { fixture, el, getRecentSearches, addRecentSearch } = render({ loggedIn: true, recent: [{ search: 'from the server' }] });
    await vi.advanceTimersByTimeAsync(200);
    fixture.detectChanges();
    expect(getRecentSearches).toHaveBeenCalled();
    expect(chips(el)).toEqual(['"from the server"']);
    type(el, 'signed in search');
    await save(fixture);
    expect(addRecentSearch).toHaveBeenCalledWith(expect.objectContaining({ search: 'signed in search' }));
    expect(stored()).toEqual([{ search: 'from this browser' }]); // the browser's list is left alone
  });

  it('waits until it knows whether the visitor is signed in', async () => {
    // auth.user() is undefined until the first check finishes.
    const getGames = vi.fn().mockReturnValue(of({ games: [], total: 0, limit: 60, offset: 0 }));
    const addRecentSearch = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: GamesService, useValue: { getGames, getFilterOptions: () => of({ genres: [], tags: [], developers: [], publishers: [], languages: [] }) } },
        { provide: AuthService, useValue: { user: () => undefined, isLoggedIn: () => false } },
        { provide: AccountService, useValue: { getRecentSearches: vi.fn(), addRecentSearch } },
        { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap({ search: 'early' }) } } },
      ],
    });
    TestBed.createComponent(SearchComponent).detectChanges();
    await vi.advanceTimersByTimeAsync(2500);
    expect(localStorage.getItem(LOCAL_RECENT_KEY)).toBeNull(); // not kept in the browser for someone who may be signed in
    expect(addRecentSearch).not.toHaveBeenCalled();
  });
});
