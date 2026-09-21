import { Component, computed, inject, PLATFORM_ID, signal } from '@angular/core';
import { isPlatformBrowser, CurrencyPipe, DatePipe } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { toObservable, toSignal } from '@angular/core/rxjs-interop';
import { switchMap, catchError, map, of } from 'rxjs';
import { ChartConfiguration } from 'chart.js';
import { BaseChartDirective } from 'ng2-charts';

import { GamesService } from '../../services/games';
import { GameCardComponent } from '../../components/game-card/game-card';
import { Game, PricePoint, Review } from '../../models/game';

type RangeKey = '1w' | '1m' | '3m' | '6m' | '1y' | '2y';
type DetailTab = 'description' | 'tags' | 'misc';

const RANGE_DAYS: Record<RangeKey, number> = {
  '1w': 7,
  '1m': 30,
  '3m': 90,
  '6m': 180,
  '1y': 365,
  '2y': 730,
};

const RANGE_LABELS: Record<RangeKey, string> = {
  '1w': 'Week',
  '1m': 'Month',
  '3m': '3 Months',
  '6m': '6 Months',
  '1y': '1 Year',
  '2y': '2 Years',
};

@Component({
  selector: 'app-game-detail',
  imports: [CurrencyPipe, DatePipe, BaseChartDirective, GameCardComponent],
  templateUrl: './game-detail.html',
  styleUrl: './game-detail.css',
})
export class GameDetailComponent {
  private route = inject(ActivatedRoute);
  private gamesService = inject(GamesService);
  protected isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  protected rangeKeys = Object.keys(RANGE_DAYS) as RangeKey[];
  protected rangeLabels = RANGE_LABELS;
  protected activeRangeTab = signal<RangeKey>('1y');
  protected activeDetailTab = signal<DetailTab>('description');
  protected selectedReview = signal<Review | null>(null);

  game = toSignal(
    this.route.paramMap.pipe(
      switchMap((params) => {
        const appId = params.get('app_id');
        if (!appId) {
          return of(null as Game | null);
        }
        return this.gamesService.getGame(Number(appId)).pipe(
          catchError((error) => {
            console.error('Error fetching game details:', error);
            return of(null as Game | null);
          })
        );
      })
    ),
    { initialValue: null }
  );

  private priceHistory = toSignal(
    this.route.paramMap.pipe(
      switchMap((params) => {
        const appId = params.get('app_id');
        if (!appId) {
          return of([] as PricePoint[]);
        }
        return this.gamesService.getPriceHistory(Number(appId)).pipe(
          catchError((error) => {
            console.error('Error fetching price history:', error);
            return of([] as PricePoint[]);
          })
        );
      })
    ),
    { initialValue: [] as PricePoint[] }
  );

  protected filteredPriceHistory = computed(() => {
    const days = RANGE_DAYS[this.activeRangeTab()];
    const cutoff = Date.now() - days * 24 * 60 * 60 * 1000;
    return this.priceHistory().filter((point) => new Date(point.date).getTime() >= cutoff);
  });

  hasPriceHistory = computed(() => this.filteredPriceHistory().length > 1);

  chartData = computed<ChartConfiguration<'line'>['data']>(() => ({
    labels: this.filteredPriceHistory().map((point) => point.date),
    datasets: [
      {
        label: 'Price (USD)',
        data: this.filteredPriceHistory().map((point) => point.price),
        borderColor: '#2563eb',
        backgroundColor: 'rgba(37, 99, 235, 0.1)',
        fill: true,
        tension: 0.2,
        pointRadius: 0,
      },
    ],
  }));

  chartOptions: ChartConfiguration<'line'>['options'] = {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      x: { ticks: { maxTicksLimit: 8 } },
      y: { ticks: { callback: (value) => `$${value}` } },
    },
    plugins: {
      legend: { display: false },
    },
  };

  protected sameDeveloperGames = toSignal(
    toObservable(this.game).pipe(
      switchMap((game) => {
        if (!game || !game.developers.length) {
          return of([] as Game[]);
        }
        return this.gamesService.getGames({ developer: game.developers[0] }).pipe(
          map((response) => response.games.filter((g) => g.app_id !== game.app_id).slice(0, 10)),
          catchError(() => of([] as Game[]))
        );
      })
    ),
    { initialValue: [] as Game[] }
  );

  protected samePublisherGames = toSignal(
    toObservable(this.game).pipe(
      switchMap((game) => {
        if (!game || !game.publishers.length) {
          return of([] as Game[]);
        }
        return this.gamesService.getGames({ publisher: game.publishers[0] }).pipe(
          map((response) => response.games.filter((g) => g.app_id !== game.app_id).slice(0, 10)),
          catchError(() => of([] as Game[]))
        );
      })
    ),
    { initialValue: [] as Game[] }
  );

  protected reviews = computed(() => this.game()?.reviews ?? []);

  protected steamReviewsUrl = computed(() => {
    const game = this.game();
    return game ? `https://store.steampowered.com/app/${game.app_id}#app_reviews_hash` : '';
  });
}
