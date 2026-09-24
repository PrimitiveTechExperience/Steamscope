import { Component, computed, inject, signal } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { toObservable, toSignal } from '@angular/core/rxjs-interop';
import { catchError, debounceTime, of, switchMap } from 'rxjs';

import { GamesService } from '../../services/games';
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

  protected onMinPriceInput(event: Event) {
    const value = (event.target as HTMLInputElement).value;
    this.minPrice.set(value === '' ? null : Number(value));
  }

  protected onMaxPriceInput(event: Event) {
    const value = (event.target as HTMLInputElement).value;
    this.maxPrice.set(value === '' ? null : Number(value));
  }
}
