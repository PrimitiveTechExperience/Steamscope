import { Component, computed, input } from '@angular/core';
import { CurrencyPipe } from '@angular/common';

import { BundleValue, BundleVerdict } from '../../models/bundle';

const LABELS: Record<BundleVerdict, string> = {
  great_deal: 'Great deal',
  good_deal: 'Good deal',
  fair: 'Fair',
  poor_value: 'Poor value',
  not_enough_data: 'Not enough data',
};

const BACKGROUNDS: Record<BundleVerdict, string> = {
  great_deal: 'var(--color-discount)',
  good_deal: 'var(--color-discount)',
  fair: 'var(--color-surface-alt)',
  poor_value: 'var(--color-accent)',
  not_enough_data: 'var(--color-surface-alt)',
};

/** "Is this bundle worth it?": the bundle's price against buying its games separately. */
@Component({
  selector: 'app-bundle-value',
  imports: [CurrencyPipe],
  templateUrl: './bundle-value.html',
})
export class BundleValueComponent {
  value = input.required<BundleValue>();
  /** How many days the bundle's own price record spans. */
  historyDays = input(0);

  protected label = computed(() => LABELS[this.value().verdict]);
  protected background = computed(() => BACKGROUNDS[this.value().verdict]);
  protected color = computed(() => (this.value().verdict === 'fair' || this.value().verdict === 'not_enough_data' ? 'var(--color-text)' : '#fff'));
  protected judged = computed(() => this.value().verdict !== 'not_enough_data');

  protected unpriced = computed(() => this.value().items.filter((i) => !i.priced).length);

  /** "Saves $9.99 (33.3%)" or "Costs $10.00 more (25.0%)". */
  protected savings(amount: number, percent: number): { text: string; good: boolean } {
    const abs = Math.abs(amount).toFixed(2);
    const pct = Math.abs(percent).toFixed(1);
    if (amount > 0.004) return { text: `Saves $${abs} (${pct}%)`, good: true };
    if (amount < -0.004) return { text: `Costs $${abs} more (${pct}%)`, good: false };
    return { text: 'No difference', good: true };
  }

  protected vsSeparate = computed(() => this.savings(this.value().savings_vs_separate, this.value().savings_vs_separate_percent));
  protected vsRegular = computed(() => this.savings(this.value().savings_vs_regular, this.value().savings_vs_regular_percent));

  protected impact(n: number): string {
    return n > 0 ? `+${n}` : `${n}`;
  }
}
