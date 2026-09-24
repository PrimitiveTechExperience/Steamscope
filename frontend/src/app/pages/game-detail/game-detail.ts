import { Component, inject } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import { switchMap, catchError, of } from 'rxjs';

import { GamesService } from '../../services/games';
import { Game } from '../../models/game';

@Component({
  selector: 'app-game-detail',
  imports: [],
  templateUrl: './game-detail.html',
  styleUrl: './game-detail.css',
})
export class GameDetailComponent {
  private route = inject(ActivatedRoute);
  private gamesService = inject(GamesService);
  
  game = toSignal(
    this.route.paramMap.pipe(
      switchMap((params) => {
        const appId = params.get('app_id');
        if (appId) {
          return this.gamesService.getGame(Number(appId));
        } else {
          return of(null);
        }
      }),
      catchError((error) => {
        console.error('Error fetching game details:', error);
        return of(null);
      })
    ),
    { initialValue: null }
  );
}
