import { Component, computed, inject } from '@angular/core';
import { RouterLink } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { catchError, map, of } from 'rxjs';

import { GamesService } from '../../services/games';
import { GameCardComponent } from '../../components/game-card/game-card';
import { RevealOnScrollDirective } from '../../directives/reveal-on-scroll';
import { Game } from '../../models/game';
import { withLoading } from '../../utils/with-loading';

@Component({
  selector: 'app-landing',
  imports: [GameCardComponent, RevealOnScrollDirective, RouterLink],
  templateUrl: './landing.html',
  styleUrl: './landing.css',
})
export class LandingComponent {
  private gamesService = inject(GamesService);

  private gamesState = toSignal(
    withLoading(
      this.gamesService.getGames().pipe(
        map((response) => response.games),
        catchError(() => of([] as Game[]))
      ),
      [] as Game[]
    ),
    { initialValue: { data: [] as Game[], loading: true } }
  );

  private games = computed(() => this.gamesState().data);
  protected trendingLoading = computed(() => this.gamesState().loading);

  protected trending = computed(() => {
    const discounted = this.games().filter((g) => g.discount_percentage > 0);
    return (discounted.length > 0 ? discounted : this.games()).slice(0, 6);
  });
}
