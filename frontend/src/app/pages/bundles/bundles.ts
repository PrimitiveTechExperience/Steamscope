import { Component, computed, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { catchError, of } from 'rxjs';

import { GamesService } from '../../services/games';
import { BundleCardComponent } from '../../components/bundle-card/bundle-card';
import { Bundle } from '../../models/bundle';
import { withLoading } from '../../utils/with-loading';

@Component({
  selector: 'app-bundles',
  imports: [BundleCardComponent],
  templateUrl: './bundles.html',
})
export class BundlesComponent {
  private state = toSignal(
    withLoading(
      inject(GamesService).getBundles().pipe(catchError(() => of([] as Bundle[]))),
      [] as Bundle[]
    ),
    { initialValue: { data: [] as Bundle[], loading: true } }
  );

  protected bundles = computed(() => this.state().data);
  protected loading = computed(() => this.state().loading);
}
