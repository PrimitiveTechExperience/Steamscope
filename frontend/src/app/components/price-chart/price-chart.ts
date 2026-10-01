import { Component, PLATFORM_ID, computed, inject, input, signal } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { ChartConfiguration } from 'chart.js';
import { BaseChartDirective } from 'ng2-charts';

import { PricePoint } from '../../models/game';
import { ThemeService } from '../../services/theme';

type RangeKey = '1w' | '1m' | '3m' | '6m' | '1y' | '2y';

const RANGE_DAYS: Record<RangeKey, number> = { '1w': 7, '1m': 30, '3m': 90, '6m': 180, '1y': 365, '2y': 730 };
const RANGE_LABELS: Record<RangeKey, string> = {
  '1w': 'Week',
  '1m': 'Month',
  '3m': '3 Months',
  '6m': '6 Months',
  '1y': '1 Year',
  '2y': '2 Years',
};

// Chart labels are user-facing text, not data - normalize the raw ISO
// timestamps from the API into something nobody has to mentally parse.
function formatChartDate(iso: string, range: RangeKey): string {
  const showYear = range === '2y' || range === '1y';
  return new Date(iso).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: showYear ? 'numeric' : undefined,
  });
}

/** Price-over-time line chart with range tabs, shared by game and bundle pages. */
@Component({
  selector: 'app-price-chart',
  imports: [BaseChartDirective],
  templateUrl: './price-chart.html',
})
export class PriceChartComponent {
  points = input.required<PricePoint[]>();
  loading = input(false);
  title = input('Price History');
  emptyMessage = input('Not enough price history for this range yet.');

  private themeService = inject(ThemeService);
  protected isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  protected rangeKeys = Object.keys(RANGE_DAYS) as RangeKey[];
  protected rangeLabels = RANGE_LABELS;
  protected activeRange = signal<RangeKey>('1y');

  private filtered = computed(() => {
    const cutoff = Date.now() - RANGE_DAYS[this.activeRange()] * 24 * 60 * 60 * 1000;
    return this.points().filter((point) => new Date(point.date).getTime() >= cutoff);
  });

  protected hasData = computed(() => this.filtered().length > 1);

  // Same hex values as --color-accent / --color-border / --color-text-muted
  // in styles.css for each theme - Chart.js can't consume CSS vars directly.
  private palette = computed(() =>
    this.themeService.theme() === 'dark'
      ? { accent: '#ff5b3d', fill: 'rgba(255, 91, 61, 0.15)', grid: '#2c2d31', tick: '#9c9ba1', surface: '#19181c' }
      : { accent: '#ff3d1f', fill: 'rgba(255, 61, 31, 0.12)', grid: '#e2e0d8', tick: '#5b5d63', surface: '#ffffff' }
  );

  protected chartData = computed<ChartConfiguration<'line'>['data']>(() => {
    const palette = this.palette();
    return {
      labels: this.filtered().map((point) => formatChartDate(point.date, this.activeRange())),
      datasets: [
        {
          label: 'Price (USD)',
          data: this.filtered().map((point) => point.price),
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

  protected chartOptions = computed<ChartConfiguration<'line'>['options']>(() => {
    const palette = this.palette();
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
}
