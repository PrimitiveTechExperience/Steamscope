import { Component, computed, inject, signal } from '@angular/core';
import { toObservable, toSignal } from '@angular/core/rxjs-interop';
import { debounceTime, switchMap } from 'rxjs';

import { GamesService } from '../../services/games';
import { GameCardComponent } from '../../components/game-card/game-card';
import { FilterOptions, GamesResponse } from '../../models/game';

type FilterCategory = 'genres' | 'tags' | 'developers' | 'publishers' | 'languages';

const EMPTY_FILTER_OPTIONS: FilterOptions = {
  genres: [],
  tags: [],
  developers: [],
  publishers: [],
  languages: [],
};

const EMPTY_RESULTS: GamesResponse = { games: [], total: 0, limit: 20, offset: 0 };

@Component({
  selector: 'app-search',
  imports: [GameCardComponent],
  templateUrl: './search.html',
  styleUrl: './search.css',
})
export class SearchComponent {
  private gamesService = inject(GamesService);

  protected filterOptions = toSignal(this.gamesService.getFilterOptions(), {
    initialValue: EMPTY_FILTER_OPTIONS,
  });

  protected selectedGenres = signal<Set<string>>(new Set());
  protected selectedTags = signal<Set<string>>(new Set());
  protected selectedDevelopers = signal<Set<string>>(new Set());
  protected selectedPublishers = signal<Set<string>>(new Set());
  protected selectedLanguages = signal<Set<string>>(new Set());
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

  protected isSelected(category: FilterCategory, value: string): boolean {
    return this.categorySignals[category]().has(value);
  }

  private query = computed(() => ({
    genres: Array.from(this.selectedGenres()),
    tags: Array.from(this.selectedTags()),
    developers: Array.from(this.selectedDevelopers()),
    publishers: Array.from(this.selectedPublishers()),
    languages: Array.from(this.selectedLanguages()),
    minPrice: this.minPrice() ?? undefined,
    maxPrice: this.maxPrice() ?? undefined,
  }));

  protected results = toSignal(
    toObservable(this.query).pipe(
      debounceTime(300),
      switchMap((query) => this.gamesService.getGames(query))
    ),
    { initialValue: EMPTY_RESULTS }
  );

  protected onMinPriceInput(event: Event) {
    const value = (event.target as HTMLInputElement).value;
    this.minPrice.set(value === '' ? null : Number(value));
  }

  protected onMaxPriceInput(event: Event) {
    const value = (event.target as HTMLInputElement).value;
    this.maxPrice.set(value === '' ? null : Number(value));
  }
}
