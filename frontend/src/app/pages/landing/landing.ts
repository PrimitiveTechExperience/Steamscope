import { Component, computed, inject } from '@angular/core';
import { RouterLink } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { catchError, map, of } from 'rxjs';

import { GamesService } from '../../services/games';
import { GameCardComponent } from '../../components/game-card/game-card';
import { RevealOnScrollDirective } from '../../directives/reveal-on-scroll';
import { Game } from '../../models/game';

@Component({
  selector: 'app-landing',
  imports: [GameCardComponent, RevealOnScrollDirective, RouterLink],
  templateUrl: './landing.html',
  styleUrl: './landing.css',
})
export class LandingComponent {
  private gamesService = inject(GamesService);

  private games = toSignal(
    this.gamesService.getGames().pipe(
      map((response) => response.games),
      catchError(() => of([] as Game[]))
    ),
    { initialValue: [] as Game[] }
  );

  protected trending = computed(() => {
    const discounted = this.games().filter((g) => g.discount_percentage > 0);
    return (discounted.length > 0 ? discounted : this.games()).slice(0, 6);
  });
}
