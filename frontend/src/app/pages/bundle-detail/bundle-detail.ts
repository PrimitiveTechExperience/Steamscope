import { Component, computed, inject, signal } from '@angular/core';
import { CurrencyPipe } from '@angular/common';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { catchError, of, switchMap } from 'rxjs';

import { GamesService } from '../../services/games';
import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { apiErrorMessage } from '../../api';
import { BundleArtComponent } from '../../components/bundle-art/bundle-art';
import { BundleValueComponent } from '../../components/bundle-value/bundle-value';
import { PriceChartComponent } from '../../components/price-chart/price-chart';
import { BundleDetail, BundleGame } from '../../models/bundle';
import { SubmissionStatus } from '../../models/user';
import { withLoading } from '../../utils/with-loading';

@Component({
  selector: 'app-bundle-detail',
  imports: [CurrencyPipe, RouterLink, PriceChartComponent, BundleArtComponent, BundleValueComponent],
  templateUrl: './bundle-detail.html',
})
export class BundleDetailComponent {
  private route = inject(ActivatedRoute);
  private gamesService = inject(GamesService);
  private account = inject(AccountService);
  protected auth = inject(AuthService);

  private state = toSignal(
    this.route.paramMap.pipe(
      switchMap((params) =>
        withLoading(
          this.gamesService.getBundle(Number(params.get('bundle_id'))).pipe(
            catchError(() => of(null as BundleDetail | null))
          ),
          null as BundleDetail | null
        )
      )
    ),
    { initialValue: { data: null as BundleDetail | null, loading: true } }
  );

  protected bundle = computed(() => this.state().data);
  protected loading = computed(() => this.state().loading);
  protected history = computed(() => this.bundle()?.price_history ?? []);
  protected games = computed(() => this.bundle()?.games ?? []);

  /** Requests made from this page, so rows update without a reload. */
  private requested = signal<Record<number, SubmissionStatus>>({});
  protected requestError = signal<string | null>(null);

  protected requestStatus(game: BundleGame): SubmissionStatus | '' {
    return this.requested()[game.app_id] ?? game.track_status;
  }

  protected statusLabel(status: SubmissionStatus | ''): string {
    switch (status) {
      case 'awaiting_approval':
        return 'Awaiting approval';
      case 'pending':
        return 'Being added...';
      case 'rejected':
        return 'Not approved';
      case 'tracked':
        return 'Added';
      default:
        return '';
    }
  }

  /** Suggests a game from this bundle for tracking (an admin must approve it). */
  protected requestGame(game: BundleGame) {
    this.requestError.set(null);
    this.account.submitGame(`https://store.steampowered.com/app/${game.app_id}`).subscribe({
      next: (res) => this.requested.update((r) => ({ ...r, [game.app_id]: res.status })),
      error: (err) => this.requestError.set(apiErrorMessage(err, "Couldn't submit that game.")),
    });
  }
}
