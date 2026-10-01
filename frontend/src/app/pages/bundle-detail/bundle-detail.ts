import { Component, computed, inject } from '@angular/core';
import { CurrencyPipe } from '@angular/common';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { catchError, of, switchMap } from 'rxjs';

import { GamesService } from '../../services/games';
import { BundleArtComponent } from '../../components/bundle-art/bundle-art';
import { PriceChartComponent } from '../../components/price-chart/price-chart';
import { BundleDetail } from '../../models/bundle';
import { withLoading } from '../../utils/with-loading';

@Component({
  selector: 'app-bundle-detail',
  imports: [CurrencyPipe, RouterLink, PriceChartComponent, BundleArtComponent],
  templateUrl: './bundle-detail.html',
})
export class BundleDetailComponent {
  private route = inject(ActivatedRoute);
  private gamesService = inject(GamesService);

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
}
