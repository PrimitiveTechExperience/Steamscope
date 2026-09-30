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
import { FilterAutocompleteComponent } from '../../components/filter-autocomplete/filter-autocomplete';
import { FilterOptions, GamesResponse } from '../../models/game';
import { withLoading } from '../../utils/with-loading';

type FilterCategory = 'genres' | 'tags' | 'developers' | 'publishers' | 'languages';

const EMPTY_FILTER_OPTIONS: FilterOptions = {
  genres: [],
  tags: [],
  developers: [],
  publishers: [],
  languages: [],
};

const EMPTY_RESULTS: GamesResponse = { games: [], total: 0, limit: 20, offset: 0 };

function isEmptySearch(s: RecentSearch): boolean {
  const lists = [s.genres, s.tags, s.developers, s.publishers, s.languages];
  return lists.every((l) => !l?.length) && s.min_price == null && s.max_price == null;
}

/** Short human-readable summary of a saved filter set, e.g. "Action, RPG · $5-$20". */
function searchLabel(s: RecentSearch): string {
  const terms = [...(s.genres ?? []), ...(s.tags ?? []), ...(s.developers ?? []), ...(s.publishers ?? []), ...(s.languages ?? [])];
  const parts: string[] = [];
  if (terms.length) {
    parts.push(terms.length > 3 ? `${terms.slice(0, 3).join(', ')} +${terms.length - 3}` : terms.join(', '));
  }
  if (s.min_price != null || s.max_price != null) {
    parts.push(`$${s.min_price ?? 0}-${s.max_price != null ? '$' + s.max_price : 'any'}`);
  }
  return parts.join(' · ');
}

function seedFromQueryParam(route: ActivatedRoute, key: string): Set<string> {
  const value = route.snapshot.queryParamMap.get(key);
  if (!value) return new Set();
  return new Set(value.split(',').map((v) => v.trim()).filter(Boolean));
}

@Component({
  selector: 'app-search',
  imports: [GameCardComponent, FilterAutocompleteComponent],
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
    genres: Array.from(this.selectedGenres()),
    tags: Array.from(this.selectedTags()),
    developers: Array.from(this.selectedDevelopers()),
    publishers: Array.from(this.selectedPublishers()),
    languages: Array.from(this.selectedLanguages()),
    minPrice: this.minPrice() ?? undefined,
    maxPrice: this.maxPrice() ?? undefined,
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

  protected results = computed(() => this.resultsState().data);
  protected resultsLoading = computed(() => this.resultsState().loading);

  // Recent searches (logged-in users). A filter set is only saved once it
  // has been left alone for a couple of seconds, so typing "100" into the
  // price box doesn't record "1", "10" and "100" as three searches.
  private auth = inject(AuthService);
  private account = inject(AccountService);
  protected recentSearches = signal<RecentSearch[]>([]);

  constructor() {
    if (!isPlatformBrowser(inject(PLATFORM_ID))) return;

    effect(() => {
      if (!this.auth.user()) {
        this.recentSearches.set([]);
        return;
      }
      untracked(() =>
        this.account.getRecentSearches().subscribe({
          next: (list) => this.recentSearches.set(list),
          error: () => {},
        })
      );
    });

    toObservable(this.query)
      .pipe(
        debounceTime(2000),
        filter(() => this.auth.isLoggedIn()),
        takeUntilDestroyed()
      )
      .subscribe((query) => {
        const search: RecentSearch = {
          genres: query.genres,
          tags: query.tags,
          developers: query.developers,
          publishers: query.publishers,
          languages: query.languages,
          min_price: query.minPrice,
          max_price: query.maxPrice,
        };
        if (isEmptySearch(search)) return;
        this.account.addRecentSearch(search).subscribe({
          next: () => this.recentSearches.update((list) =>
            [search, ...list.filter((s) => searchLabel(s) !== searchLabel(search))].slice(0, 10)
          ),
          error: () => {},
        });
      });
  }

  protected applyRecent(search: RecentSearch) {
    this.selectedGenres.set(new Set(search.genres ?? []));
    this.selectedTags.set(new Set(search.tags ?? []));
    this.selectedDevelopers.set(new Set(search.developers ?? []));
    this.selectedPublishers.set(new Set(search.publishers ?? []));
    this.selectedLanguages.set(new Set(search.languages ?? []));
    this.minPrice.set(search.min_price ?? null);
    this.maxPrice.set(search.max_price ?? null);
  }

  protected searchLabel = searchLabel;

  protected onMinPriceInput(event: Event) {
    const value = (event.target as HTMLInputElement).value;
    this.minPrice.set(value === '' ? null : Number(value));
  }

  protected onMaxPriceInput(event: Event) {
    const value = (event.target as HTMLInputElement).value;
    this.maxPrice.set(value === '' ? null : Number(value));
  }
}
