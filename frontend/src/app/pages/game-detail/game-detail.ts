import { Component, afterNextRender, computed, inject, PLATFORM_ID, signal } from '@angular/core';
import { isPlatformBrowser, CurrencyPipe, DatePipe, DecimalPipe } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { toObservable, toSignal } from '@angular/core/rxjs-interop';
import { switchMap, catchError, map, of } from 'rxjs';
import { ChartConfiguration } from 'chart.js';
import { BaseChartDirective } from 'ng2-charts';

import { GamesService } from '../../services/games';
import { GameCardComponent } from '../../components/game-card/game-card';
import { RevealOnScrollDirective } from '../../directives/reveal-on-scroll';
import { TiltDirective } from '../../directives/tilt';
import { GrowOnScrollDirective } from '../../directives/grow-on-scroll';
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
  imports: [CurrencyPipe, DatePipe, DecimalPipe, BaseChartDirective, GameCardComponent, RevealOnScrollDirective, TiltDirective, GrowOnScrollDirective],
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

  // Chart.js needs literal color strings, not CSS custom properties, so the
  // chart can't just read var(--color-accent) - it has to track the
  // data-theme attribute itself to stay in sync with the header's toggle.
  private themeAttr = signal<'light' | 'dark'>('light');

  constructor() {
    afterNextRender(() => {
      if (!this.isBrowser) return;
      const root = document.documentElement;
      const readTheme = () => (root.getAttribute('data-theme') === 'dark' ? 'dark' : 'light');
      this.themeAttr.set(readTheme());
      const observer = new MutationObserver(() => this.themeAttr.set(readTheme()));
      observer.observe(root, { attributes: true, attributeFilter: ['data-theme'] });
    });
  }

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

  // Same hex values as --color-accent / --color-border / --color-text-muted
  // in styles.css for each theme - Chart.js can't consume CSS vars directly.
  private chartPalette = computed(() =>
    this.themeAttr() === 'dark'
      ? { accent: '#ff5b3d', fill: 'rgba(255, 91, 61, 0.15)', grid: '#2c2d31', tick: '#9c9ba1', surface: '#19181c' }
      : { accent: '#ff3d1f', fill: 'rgba(255, 61, 31, 0.12)', grid: '#e2e0d8', tick: '#5b5d63', surface: '#ffffff' }
  );

  chartData = computed<ChartConfiguration<'line'>['data']>(() => {
    const palette = this.chartPalette();
    return {
      labels: this.filteredPriceHistory().map((point) => point.date),
      datasets: [
        {
          label: 'Price (USD)',
          data: this.filteredPriceHistory().map((point) => point.price),
          borderColor: palette.accent,
          backgroundColor: palette.fill,
          fill: true,
          tension: 0,
          borderWidth: 2,
          pointStyle: 'rect',
          pointRadius: 0,
          pointHoverRadius: 5,
          pointBackgroundColor: palette.accent,
        },
      ],
    };
  });

  chartOptions = computed<ChartConfiguration<'line'>['options']>(() => {
    const palette = this.chartPalette();
    return {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: {
          ticks: { maxTicksLimit: 8, color: palette.tick, font: { family: 'Space Grotesk' } },
          grid: { color: palette.grid },
          border: { color: palette.grid },
        },
        y: {
          ticks: { callback: (value) => `$${value}`, color: palette.tick, font: { family: 'Space Grotesk' } },
          grid: { color: palette.grid },
          border: { color: palette.grid },
        },
      },
      plugins: {
        legend: { display: false },
        tooltip: {
          backgroundColor: palette.surface,
          titleColor: palette.tick,
          bodyColor: palette.accent,
          borderColor: palette.grid,
          borderWidth: 2,
          cornerRadius: 0,
          padding: 10,
          displayColors: false,
        },
      },
    };
  });

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
