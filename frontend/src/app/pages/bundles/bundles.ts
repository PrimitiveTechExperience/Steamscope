import { Component, computed, inject, signal } from '@angular/core';
import { toObservable, toSignal } from '@angular/core/rxjs-interop';
import { catchError, of, switchMap } from 'rxjs';

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
  private games = inject(GamesService);

  /** What is in the box, and what was last searched for (on Enter or the button). */
  protected searchTerm = signal('');
  protected activeSearch = signal('');

  private state = toSignal(
    toObservable(this.activeSearch).pipe(
      switchMap((search) =>
        withLoading(this.games.getBundles(undefined, search).pipe(catchError(() => of([] as Bundle[]))), [] as Bundle[])
      )
    ),
    { initialValue: { data: [] as Bundle[], loading: true } }
  );

  protected bundles = computed(() => this.state().data);
  protected loading = computed(() => this.state().loading);

  protected onInput(event: Event) {
    this.searchTerm.set((event.target as HTMLInputElement).value);
  }

  protected search() {
    this.activeSearch.set(this.searchTerm().trim());
  }
}
