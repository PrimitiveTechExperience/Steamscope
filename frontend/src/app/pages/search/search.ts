import { Component, PLATFORM_ID, computed, effect, inject, signal, untracked } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { takeUntilDestroyed, toObservable, toSignal } from '@angular/core/rxjs-interop';
import { catchError, debounceTime, filter, of, switchMap } from 'rxjs';

import { GamesService } from '../../services/games';
import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { RecentSearch } from '../../models/user';
import { GameCardComponent } from '../../components/game-card/game-card';
import { BundleCardComponent } from '../../components/bundle-card/bundle-card';
import { FilterAutocompleteComponent } from '../../components/filter-autocomplete/filter-autocomplete';
import { FilterOptions, GamesResponse } from '../../models/game';
import { withLoading } from '../../utils/with-loading';
import { loadLocalRecentSearches, saveLocalRecentSearches, tidyRecentSearch } from '../../utils/local-recent-searches';
import { discountOf } from '../../utils/discount';

type FilterCategory = 'genres' | 'tags' | 'developers' | 'publishers' | 'languages';

const EMPTY_FILTER_OPTIONS: FilterOptions = {
  genres: [],
  tags: [],
  developers: [],
  publishers: [],
  languages: [],
};

const EMPTY_RESULTS: GamesResponse = { games: [], total: 0, limit: 20, offset: 0 };

/** How many recent searches are shown (the server, and this browser for signed-out visitors, keep the same number). */
export const MAX_RECENT_SEARCHES = 5;

/** A recent-search chip is cut to this many characters, so a long search cannot stretch the row. */
export const MAX_CHIP_LABEL = 50;

/** Cuts a label to `max` characters, ending in an ellipsis when it was longer. */
export function shortLabel(label: string, max = MAX_CHIP_LABEL): string {
  const chars = Array.from(label); // count characters, not UTF-16 units, so emoji are not split
  return chars.length <= max ? label : chars.slice(0, max - 1).join('').trimEnd() + '…';
}

/** The "minimum discount" choices. 1 means "on sale at all". */
export const DISCOUNT_STEPS: { label: string; value: number | null }[] = [
  { label: 'Any', value: null },
  { label: 'On sale', value: 1 },
  { label: '25%+', value: 25 },
  { label: '50%+', value: 50 },
  { label: '75%+', value: 75 },
];

function isEmptySearch(s: RecentSearch): boolean {
  const lists = [s.genres, s.tags, s.developers, s.publishers, s.languages];
  // The bundles switch on its own is not a search worth remembering.
  return lists.every((l) => !l?.length) && !s.search?.trim() && s.min_price == null && s.max_price == null && s.min_discount == null;
}

/** Short human-readable summary of a saved filter set, e.g. "Action, RPG · $5-$20". */
function searchLabel(s: RecentSearch): string {
  const text = s.search?.trim() ? `"${s.search.trim()}"` : '';
  const terms = [...(s.genres ?? []), ...(s.tags ?? []), ...(s.developers ?? []), ...(s.publishers ?? []), ...(s.languages ?? [])];
  const parts: string[] = [];
  if (text) parts.push(text);
  if (terms.length) {
    parts.push(terms.length > 3 ? `${terms.slice(0, 3).join(', ')} +${terms.length - 3}` : terms.join(', '));
  }
  if (s.min_price != null || s.max_price != null) {
    parts.push(`$${s.min_price ?? 0}-${s.max_price != null ? '$' + s.max_price : 'any'}`);
  }
  if (s.min_discount != null) {
    parts.push(s.min_discount <= 1 ? 'On sale' : `${s.min_discount}%+ off`);
  }
  if (s.include_bundles) parts.push('+ bundles');
  return parts.join(' · ');
}

function seedFromQueryParam(route: ActivatedRoute, key: string): Set<string> {
  const value = route.snapshot.queryParamMap.get(key);
  if (!value) return new Set();
  return new Set(value.split(',').map((v) => v.trim()).filter(Boolean));
}

/** /search?search=portal opens the page with that already typed. */
function seedSearchText(route: ActivatedRoute): string {
  return (route.snapshot.queryParamMap.get('search') ?? '').slice(0, 100);
}

/** /search?bundles=1 opens it with bundles included. */
function seedBundles(route: ActivatedRoute): boolean {
  const v = route.snapshot.queryParamMap.get('bundles');
  return v === '1' || v === 'true';
}

/** /search?minDiscount=50 opens the page already filtered to deals. */
function seedDiscount(route: ActivatedRoute): number | null {
  const n = Number(route.snapshot.queryParamMap.get('minDiscount'));
  return Number.isInteger(n) && n >= 1 && n <= 100 ? n : null;
}

@Component({
  selector: 'app-search',
  imports: [GameCardComponent, BundleCardComponent, FilterAutocompleteComponent],
  templateUrl: './search.html',
  styleUrl: './search.css',
})
export class SearchComponent {
  private gamesService = inject(GamesService);
  private route = inject(ActivatedRoute);

  protected filterOptions = toSignal(this.gamesService.getFilterOptions(), {
    initialValue: EMPTY_FILTER_OPTIONS,
  });

  // Pre-filled when arriving from a "View All" link on the game-detail page
  // (e.g. /search?developers=Valve).
  protected selectedGenres = signal<Set<string>>(seedFromQueryParam(this.route, 'genres'));
  protected selectedTags = signal<Set<string>>(seedFromQueryParam(this.route, 'tags'));
  protected selectedDevelopers = signal<Set<string>>(seedFromQueryParam(this.route, 'developers'));
  protected selectedPublishers = signal<Set<string>>(seedFromQueryParam(this.route, 'publishers'));
  protected selectedLanguages = signal<Set<string>>(seedFromQueryParam(this.route, 'languages'));
  protected minPrice = signal<number | null>(null);
  protected maxPrice = signal<number | null>(null);
  protected minDiscount = signal<number | null>(seedDiscount(this.route));
  protected searchText = signal(seedSearchText(this.route));
  protected includeBundles = signal(seedBundles(this.route));

  protected onSearchInput(event: Event) {
    this.searchText.set((event.target as HTMLInputElement).value);
  }

  protected onIncludeBundles(event: Event) {
    this.includeBundles.set((event.target as HTMLInputElement).checked);
  }
  protected discountSteps = DISCOUNT_STEPS;

  protected setMinDiscount(value: number | null) {
    this.minDiscount.set(value);
  }

  private categorySignals: Record<FilterCategory, ReturnType<typeof signal<Set<string>>>> = {
    genres: this.selectedGenres,
    tags: this.selectedTags,
    developers: this.selectedDevelopers,
    publishers: this.selectedPublishers,
    languages: this.selectedLanguages,
  };

  protected toggleFilter(category: FilterCategory, value: string) {
    const set = this.categorySignals[category];
    const next = new Set(set());
    if (next.has(value)) {
      next.delete(value);
    } else {
      next.add(value);
    }
    set.set(next);
  }

  // Array views for the autocomplete component, which needs to `@for` over
  // and diff selections - plain arrays are simpler to bind than Sets.
  protected selectedGenresArray = computed(() => Array.from(this.selectedGenres()));
  protected selectedTagsArray = computed(() => Array.from(this.selectedTags()));
  protected selectedDevelopersArray = computed(() => Array.from(this.selectedDevelopers()));
  protected selectedPublishersArray = computed(() => Array.from(this.selectedPublishers()));
  protected selectedLanguagesArray = computed(() => Array.from(this.selectedLanguages()));

  private query = computed(() => ({
    search: this.searchText().trim() || undefined,
    includeBundles: this.includeBundles() || undefined,
    genres: Array.from(this.selectedGenres()),
    tags: Array.from(this.selectedTags()),
    developers: Array.from(this.selectedDevelopers()),
    publishers: Array.from(this.selectedPublishers()),
    languages: Array.from(this.selectedLanguages()),
    minPrice: this.minPrice() ?? undefined,
    maxPrice: this.maxPrice() ?? undefined,
    minDiscount: this.minDiscount() ?? undefined,
    limit: 60,
  }));

  private resultsState = toSignal(
    toObservable(this.query).pipe(
      debounceTime(150),
      switchMap((query) =>
        withLoading(
          this.gamesService.getGames(query).pipe(catchError(() => of(EMPTY_RESULTS))),
          EMPTY_RESULTS
        )
      )
    ),
    { initialValue: { data: EMPTY_RESULTS, loading: true } }
  );

  /**
   * The games to show. The server applies the discount filter, but it is applied
   * here as well so the page is right even when it talks to a server that does
   * not know the filter yet (it would ignore the parameter and send everything).
   */
  protected results = computed(() => {
    const data = this.resultsState().data;
    const min = this.minDiscount();
    if (min === null) return data;
    return { ...data, games: (data.games ?? []).filter((g) => discountOf(g) >= min) };
  });

  /** The bundles to show: none unless asked for. */
  protected bundles = computed(() => (this.includeBundles() ? (this.results().bundles ?? []) : []));

  /**
   * Genre, tag, developer, publisher and language describe a game; a bundle has none
   * of its own, so the server leaves bundles out while any of them is set.
   */
  protected bundlesLeftOut = computed(
    () =>
      this.includeBundles() &&
      (this.selectedGenres().size > 0 ||
        this.selectedTags().size > 0 ||
        this.selectedDevelopers().size > 0 ||
        this.selectedPublishers().size > 0 ||
        this.selectedLanguages().size > 0)
  );

  protected emptyMessage = computed(() =>
    this.includeBundles() ? 'No games or bundles match these filters.' : 'No games match these filters.'
  );
  protected resultsLoading = computed(() => this.resultsState().loading);

  // Recent searches. Signed-in users' are kept on the server, so they follow
  // them between devices; a signed-out visitor's are kept in this browser. A
  // filter set is only saved once it has been left alone for a couple of
  // seconds, so typing "100" into the price box doesn't record "1", "10" and
  // "100" as three searches.
  private auth = inject(AuthService);
  private account = inject(AccountService);
  protected recentSearches = signal<RecentSearch[]>([]);

  constructor() {
    if (!isPlatformBrowser(inject(PLATFORM_ID))) return;

    effect(() => {
      const user = this.auth.user();
      if (user === undefined) return; // not known yet: wait, rather than show the wrong list
      if (user === null) {
        this.recentSearches.set(loadLocalRecentSearches(MAX_RECENT_SEARCHES));
        return;
      }
      untracked(() =>
        this.account.getRecentSearches().subscribe({
          next: (list) => this.recentSearches.set(list.slice(0, MAX_RECENT_SEARCHES)),
          error: () => {},
        })
      );
    });

    toObservable(this.query)
      .pipe(
        debounceTime(2000),
        // Until we know whether the visitor is signed in we don't know where to keep it.
        filter(() => this.auth.user() !== undefined),
        takeUntilDestroyed()
      )
      .subscribe((query) => {
        const search: RecentSearch = {
          search: query.search,
          include_bundles: query.includeBundles,
          genres: query.genres,
          tags: query.tags,
          developers: query.developers,
          publishers: query.publishers,
          languages: query.languages,
          min_price: query.minPrice,
          max_price: query.maxPrice,
          min_discount: query.minDiscount,
        };
        if (isEmptySearch(search)) return;
        const withThis = (list: RecentSearch[]) =>
          [search, ...list.filter((s) => searchLabel(s) !== searchLabel(search))].slice(0, MAX_RECENT_SEARCHES);
        if (!this.auth.isLoggedIn()) {
          // Keep just what was set, the same shape as what is read back.
          const stored = withThis(this.recentSearches())
            .map((s) => tidyRecentSearch(s))
            .filter((s): s is RecentSearch => s !== null);
          saveLocalRecentSearches(stored);
          this.recentSearches.set(stored);
          return;
        }
        this.account.addRecentSearch(search).subscribe({
          next: () => this.recentSearches.update(withThis),
          error: () => {},
        });
      });
  }

  protected applyRecent(search: RecentSearch) {
    this.searchText.set(search.search ?? '');
    this.includeBundles.set(search.include_bundles ?? false);
    this.selectedGenres.set(new Set(search.genres ?? []));
    this.selectedTags.set(new Set(search.tags ?? []));
    this.selectedDevelopers.set(new Set(search.developers ?? []));
    this.selectedPublishers.set(new Set(search.publishers ?? []));
    this.selectedLanguages.set(new Set(search.languages ?? []));
    this.minPrice.set(search.min_price ?? null);
    this.maxPrice.set(search.max_price ?? null);
    this.minDiscount.set(search.min_discount ?? null);
  }

  protected searchLabel = searchLabel;
  protected chipLabel = (s: RecentSearch) => shortLabel(searchLabel(s));

  protected onMinPriceInput(event: Event) {
    const value = (event.target as HTMLInputElement).value;
    this.minPrice.set(value === '' ? null : Number(value));
  }

  protected onMaxPriceInput(event: Event) {
    const value = (event.target as HTMLInputElement).value;
    this.maxPrice.set(value === '' ? null : Number(value));
  }
}
