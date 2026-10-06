import { Component, PLATFORM_ID, computed, effect, inject, input, output } from '@angular/core';
import { CurrencyPipe, DatePipe, DecimalPipe, isPlatformBrowser } from '@angular/common';
import { toObservable, toSignal } from '@angular/core/rxjs-interop';
import { ChartConfiguration } from 'chart.js';
import { BaseChartDirective } from 'ng2-charts';
import { EMPTY, Observable, catchError, combineLatest, map, of, startWith, switchMap } from 'rxjs';

import { GamesService } from '../../services/games';
import { ThemeService } from '../../services/theme';
import { Advice, Forecast, Verdict } from '../../models/prediction';
import { confidenceText } from '../../utils/confidence';

interface Load<T> {
  data: T | null;
  loading: boolean;
  failed: boolean;
}

const LOADING = { data: null, loading: true, failed: false };

function load<T>(source: Observable<T>): Observable<Load<T>> {
  return source.pipe(
    map((data) => ({ data, loading: false, failed: false })),
    catchError(() => of({ data: null, loading: false, failed: true })),
    startWith(LOADING)
  );
}

const VERDICT_LABELS: Record<Verdict, string> = {
  buy_now: 'Buy now',
  wait: 'Wait',
  toss_up: 'Toss-up',
  not_enough_data: 'Not enough data',
  free: 'Free',
};

const VERDICT_BACKGROUNDS: Record<Verdict, string> = {
  buy_now: 'var(--color-discount)',
  wait: 'var(--color-accent)',
  toss_up: 'var(--color-surface-alt)',
  not_enough_data: 'var(--color-surface-alt)',
  free: 'var(--color-discount)',
};

const HORIZON_LABELS: Record<number, string> = { 30: '1 month', 90: '3 months', 180: '6 months', 365: '1 year', 730: '2 years' };

/** "Buy now or wait?": the verdict for a game or bundle, why, and the two-year price outlook. */
@Component({
  selector: 'app-price-prediction',
  imports: [BaseChartDirective, CurrencyPipe, DatePipe, DecimalPipe],
  templateUrl: './price-prediction.html',
})
export class PricePredictionComponent {
  /** The game to forecast. Give either this or `bundleId`. */
  appId = input<number | null>(null);
  /** The bundle to forecast instead of a game. */
  bundleId = input<number | null>(null);
  /** Change this to refetch, e.g. after the user starts watching or sets a target price. */
  refreshKey = input<unknown>(null);

  /** Emits whether today's price is the lowest on record, once the forecast has loaded. */
  recordLowChange = output<boolean>();

  private games = inject(GamesService);
  private themeService = inject(ThemeService);
  protected isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  private state = toSignal(
    combineLatest([toObservable(this.appId), toObservable(this.bundleId), toObservable(this.refreshKey)]).pipe(
      switchMap(([appId, bundleId]) => {
        const [prediction, advice] =
          bundleId != null
            ? [this.games.getBundlePrediction(bundleId), this.games.getBundleAdvice(bundleId)]
            : appId != null
              ? [this.games.getPrediction(appId), this.games.getAdvice(appId)]
              : [EMPTY as Observable<Forecast>, EMPTY as Observable<Advice>];
        return combineLatest([load(prediction), load(advice)]).pipe(map(([forecast, advice]) => ({ forecast, advice })));
      })
    ),
    { initialValue: { forecast: LOADING as Load<Forecast>, advice: LOADING as Load<Advice> } }
  );

  protected isBundle = computed(() => this.bundleId() != null);
  /** What the forecast is about, for the wording in the template. */
  protected subject = computed(() => (this.isBundle() ? 'bundle' : 'game'));

  protected forecast = computed(() => this.state().forecast.data);
  protected advice = computed(() => this.state().advice.data);
  protected loading = computed(() => this.state().forecast.loading || this.state().advice.loading);
  protected failed = computed(() => this.state().forecast.failed);

  protected hasForecast = computed(() => {
    const f = this.forecast();
    return !!f && f.model !== 'insufficient' && f.model !== 'free';
  });
  protected isFree = computed(() => this.forecast()?.model === 'free');
  protected atRecordLow = computed(() => this.forecast()?.at_record_low === true);

  constructor() {
    effect(() => this.recordLowChange.emit(this.atRecordLow()));
  }

  protected verdictLabel = computed(() => VERDICT_LABELS[this.advice()?.verdict ?? 'not_enough_data']);
  protected verdictBackground = computed(() => VERDICT_BACKGROUNDS[this.advice()?.verdict ?? 'not_enough_data']);
  protected verdictColor = computed(() => {
    const v = this.advice()?.verdict;
    return v === 'buy_now' || v === 'wait' || v === 'free' ? '#fff' : 'var(--color-text)';
  });

  // e.g. "High-medium (54.37%)": the word, plus the exact percentage.
  protected forecastConfidence = computed(() => confidenceText(this.forecast()?.confidence ?? 0));
  protected adviceConfidence = computed(() => confidenceText(this.advice()?.confidence ?? 0));

  protected nextSaleText = computed(() => {
    const next = this.forecast()?.next_sale;
    if (!next || next.median_days === null) return 'No sale is expected within two years';
    const range = next.p25_days !== null && next.p75_days !== null ? ` (likely ${next.p25_days} to ${next.p75_days} days)` : '';
    return `Next sale expected in about ${next.median_days} ${next.median_days === 1 ? 'day' : 'days'}${range}`;
  });

  protected horizons = computed(() =>
    (this.forecast()?.horizons ?? []).map((h) => ({
      label: HORIZON_LABELS[h.days] ?? `${h.days} days`,
      chance: Math.round(100 * h.p_lower),
      expectedPrice: h.expected_price,
      expectedLow: h.expected_low,
    }))
  );

  protected typicalSaleText = computed(() => {
    const t = this.forecast()?.typical_sale;
    if (!t || t.count === 0) return 'No past sales seen';
    return `${t.count} past ${t.count === 1 ? 'sale' : 'sales'}, usually ${Math.round(t.median_depth_percent)}% off for about ${t.median_duration_days} ${t.median_duration_days === 1 ? 'day' : 'days'}`;
  });

  protected impact(value: number): string {
    return value > 0 ? `+${value}` : `${value}`;
  }

  // Same hex values as the price chart, for each theme.
  private palette = computed(() =>
    this.themeService.theme() === 'dark'
      ? { accent: '#ff5b3d', fill: 'rgba(255, 91, 61, 0.12)', second: '#6cc7a1', grid: '#2c2d31', tick: '#9c9ba1', surface: '#19181c' }
      : { accent: '#ff3d1f', fill: 'rgba(255, 61, 31, 0.10)', second: '#1f8f64', grid: '#e2e0d8', tick: '#5b5d63', surface: '#ffffff' }
  );

  protected chartData = computed<ChartConfiguration<'line'>['data']>(() => {
    const curve = this.forecast()?.curve ?? [];
    const p = this.palette();
    return {
      labels: curve.map((c) => formatMonth(c.date)),
      datasets: [
        {
          label: 'Expected price',
          data: curve.map((c) => c.expected_price),
          yAxisID: 'y',
          borderColor: p.accent,
          backgroundColor: p.fill,
          fill: true,
          tension: 0,
          borderWidth: 2,
          pointRadius: 0,
          pointHoverRadius: 4,
        },
        {
          label: 'Chance of a lower price (%)',
          data: curve.map((c) => Math.round(100 * c.p_lower_by)),
          yAxisID: 'y1',
          borderColor: p.second,
          borderDash: [6, 4],
          fill: false,
          tension: 0,
          borderWidth: 2,
          pointRadius: 0,
          pointHoverRadius: 4,
        },
      ],
    };
  });

  protected chartOptions = computed<ChartConfiguration<'line'>['options']>(() => {
    const p = this.palette();
    const ticks = { color: p.tick, font: { family: 'Space Grotesk' } };
    return {
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: { ticks: { ...ticks, maxTicksLimit: 8 }, grid: { color: p.grid }, border: { color: p.grid } },
        y: { position: 'left', ticks: { ...ticks, callback: (v) => `$${v}` }, grid: { color: p.grid }, border: { color: p.grid } },
        y1: { position: 'right', min: 0, max: 100, ticks: { ...ticks, callback: (v) => `${v}%` }, grid: { drawOnChartArea: false }, border: { color: p.grid } },
      },
      plugins: {
        legend: { display: true, labels: { color: p.tick, font: { family: 'Space Grotesk' }, boxWidth: 12 } },
        tooltip: { backgroundColor: p.surface, titleColor: p.tick, bodyColor: p.accent, borderColor: p.grid, borderWidth: 2, cornerRadius: 0, padding: 10 },
      },
    };
  });
}

/** "2027-03-14" -> "Mar '27". Dates are calendar days, so they are formatted in UTC. */
function formatMonth(date: string): string {
  const d = new Date(`${date}T00:00:00Z`);
  const month = d.toLocaleDateString('en-US', { month: 'short', timeZone: 'UTC' });
  return `${month} '${String(d.getUTCFullYear()).slice(2)}`;
}
