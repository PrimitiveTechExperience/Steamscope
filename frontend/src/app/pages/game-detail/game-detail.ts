import { Component, computed, effect, inject, PLATFORM_ID, signal, untracked } from '@angular/core';
import { isPlatformBrowser, CurrencyPipe, DatePipe, DecimalPipe } from '@angular/common';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { toObservable, toSignal } from '@angular/core/rxjs-interop';
import { switchMap, catchError, map, of } from 'rxjs';

import { GamesService } from '../../services/games';
import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { apiErrorMessage } from '../../api';
import { PriceChartComponent } from '../../components/price-chart/price-chart';
import { BundleCardComponent } from '../../components/bundle-card/bundle-card';
import { PricePredictionComponent } from '../../components/price-prediction/price-prediction';
import { GameCardComponent } from '../../components/game-card/game-card';
import { RevealOnScrollDirective } from '../../directives/reveal-on-scroll';
import { TiltDirective } from '../../directives/tilt';
import { GrowOnScrollDirective } from '../../directives/grow-on-scroll';
import { Bundle } from '../../models/bundle';
import { Game, PricePoint, Review } from '../../models/game';
import { withLoading } from '../../utils/with-loading';
import { onHeaderImageError } from '../../utils/steam-image';

type DetailTab = 'description' | 'tags' | 'misc';

@Component({
  selector: 'app-game-detail',
  imports: [CurrencyPipe, DatePipe, DecimalPipe, PriceChartComponent, PricePredictionComponent, BundleCardComponent, GameCardComponent, RevealOnScrollDirective, TiltDirective, GrowOnScrollDirective, RouterLink, FormsModule],
  templateUrl: './game-detail.html',
  styleUrl: './game-detail.css',
})
export class GameDetailComponent {
  private route = inject(ActivatedRoute);
  private gamesService = inject(GamesService);
  protected auth = inject(AuthService);
  private account = inject(AccountService);
  protected isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  protected activeDetailTab = signal<DetailTab>('description');
  protected selectedReview = signal<Review | null>(null);
  protected onImageError = onHeaderImageError;

  /** null until known (or when logged out). */
  protected watchState = signal<{ watched: boolean; pinned: boolean } | null>(null);
  /** Set by the price-prediction panel once it knows the lowest price on record. */
  protected isRecordLow = signal(false);
  protected targetPriceInput: number | null = null;
  protected watchError = signal<string | null>(null);

  constructor() {
    // Once both the game and the logged-in user are known, look up whether
    // this game is on the user's watchlist.
    effect(() => {
      const game = this.game();
      const user = this.auth.user();
      if (!this.isBrowser || !game || !user) {
        this.watchState.set(null);
        return;
      }
      untracked(() =>
        this.account.getWatchlist().subscribe({
          next: (list) => {
            const entry = list.find((w) => w.game.app_id === game.app_id);
            this.watchState.set({ watched: !!entry, pinned: entry?.pinned ?? false });
            this.targetPriceInput = entry?.target_price ?? null;
          },
          error: () => this.watchState.set(null),
        })
      );
    });
  }

  protected toggleWatch() {
    const game = this.game();
    const state = this.watchState();
    if (!game || !state) return;
    const request = state.watched
      ? this.account.unwatch(game.app_id)
      : this.account.watch(game.app_id, false, null);
    request.subscribe({
      next: () => {
        this.watchState.set({ watched: !state.watched, pinned: false });
        if (state.watched) this.targetPriceInput = null;
        this.watchError.set(null);
      },
      error: (err) => this.watchError.set(apiErrorMessage(err, "Couldn't update your watchlist.")),
    });
  }

  protected togglePin() {
    const game = this.game();
    const state = this.watchState();
    if (!game || !state) return;
    this.account.watch(game.app_id, !state.pinned, this.targetPriceInput).subscribe({
      next: () => {
        this.watchState.set({ watched: true, pinned: !state.pinned });
        this.watchError.set(null);
      },
      error: (err) => this.watchError.set(apiErrorMessage(err, "Couldn't update your watchlist.")),
    });
  }

  protected saveTargetPrice() {
    const game = this.game();
    const state = this.watchState();
    if (!game || !state) return;
    this.account.watch(game.app_id, state.pinned, this.targetPriceInput).subscribe({
      next: () => {
        this.watchState.set({ watched: true, pinned: state.pinned });
        this.watchError.set(null);
      },
      error: (err) => this.watchError.set(apiErrorMessage(err, "Couldn't save your target price.")),
    });
  }

  private gameState = toSignal(
    this.route.paramMap.pipe(
      switchMap((params) => {
        const appId = params.get('app_id');
        if (!appId) {
          return of({ data: null as Game | null, loading: false });
        }
        return withLoading(
          this.gamesService.getGame(Number(appId)).pipe(
            catchError((error) => {
              console.error('Error fetching game details:', error);
              return of(null as Game | null);
            })
          ),
          null as Game | null
        );
      })
    ),
    { initialValue: { data: null as Game | null, loading: true } }
  );

  game = computed(() => this.gameState().data);
  protected gameLoading = computed(() => this.gameState().loading);

  private priceHistoryState = toSignal(
    this.route.paramMap.pipe(
      switchMap((params) => {
        const appId = params.get('app_id');
        if (!appId) {
          return of({ data: [] as PricePoint[], loading: false });
        }
        return withLoading(
          this.gamesService.getPriceHistory(Number(appId)).pipe(
            catchError((error) => {
              console.error('Error fetching price history:', error);
              return of([] as PricePoint[]);
            })
          ),
          [] as PricePoint[]
        );
      })
    ),
    { initialValue: { data: [] as PricePoint[], loading: true } }
  );

  protected priceHistory = computed(() => this.priceHistoryState().data);
  protected priceHistoryLoading = computed(() => this.priceHistoryState().loading);

  private bundlesState = toSignal(
    this.route.paramMap.pipe(
      switchMap((params) => {
        const appId = params.get('app_id');
        if (!appId) return of([] as Bundle[]);
        return this.gamesService.getBundles(Number(appId)).pipe(catchError(() => of([] as Bundle[])));
      })
    ),
    { initialValue: [] as Bundle[] }
  );

  protected bundles = computed(() => this.bundlesState());

  private sameDeveloperState = toSignal(
    toObservable(this.game).pipe(
      switchMap((game) => {
        if (!game || !game.developers.length) {
          return of({ data: [] as Game[], loading: false });
        }
        return withLoading(
          this.gamesService.getGames({ developer: game.developers[0] }).pipe(
            map((response) => response.games.filter((g) => g.app_id !== game.app_id).slice(0, 10)),
            catchError(() => of([] as Game[]))
          ),
          [] as Game[]
        );
      })
    ),
    { initialValue: { data: [] as Game[], loading: false } }
  );

  protected sameDeveloperGames = computed(() => this.sameDeveloperState().data);
  protected sameDeveloperLoading = computed(() => this.sameDeveloperState().loading);

  private samePublisherState = toSignal(
    toObservable(this.game).pipe(
      switchMap((game) => {
        if (!game || !game.publishers.length) {
          return of({ data: [] as Game[], loading: false });
        }
        return withLoading(
          this.gamesService.getGames({ publisher: game.publishers[0] }).pipe(
            map((response) => response.games.filter((g) => g.app_id !== game.app_id).slice(0, 10)),
            catchError(() => of([] as Game[]))
          ),
          [] as Game[]
        );
      })
    ),
    { initialValue: { data: [] as Game[], loading: false } }
  );

  protected samePublisherGames = computed(() => this.samePublisherState().data);
  protected samePublisherLoading = computed(() => this.samePublisherState().loading);

  protected reviews = computed(() => this.game()?.reviews ?? []);

  protected steamReviewsUrl = computed(() => {
    const game = this.game();
    return game ? `https://store.steampowered.com/app/${game.app_id}#app_reviews_hash` : '';
  });
}
