import { Component, computed, inject, PLATFORM_ID } from '@angular/core';
import { isPlatformBrowser, CurrencyPipe } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { switchMap, catchError, of } from 'rxjs';
import { ChartConfiguration } from 'chart.js';
import { BaseChartDirective } from 'ng2-charts';

import { GamesService } from '../../services/games';
import { Game, PricePoint } from '../../models/game';

@Component({
  selector: 'app-game-detail',
  imports: [CurrencyPipe, BaseChartDirective],
  templateUrl: './game-detail.html',
  styleUrl: './game-detail.css',
})
export class GameDetailComponent {
  private route = inject(ActivatedRoute);
  private gamesService = inject(GamesService);
  protected isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

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

  hasPriceHistory = computed(() => this.priceHistory().length > 1);

  chartData = computed<ChartConfiguration<'line'>['data']>(() => ({
    labels: this.priceHistory().map((point) => point.date),
    datasets: [
      {
        label: 'Price (USD)',
        data: this.priceHistory().map((point) => point.price),
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
}
