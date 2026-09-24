import { Component, inject} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { catchError, map, of } from 'rxjs';

import {GamesService} from '../../services/games';
import {Game} from '../../models/game';
import { GameCardComponent } from '../../components/game-card/game-card';
import { FormsModule } from '@angular/forms';

@Component({
  selector: 'app-games',
  imports: [GameCardComponent, FormsModule],
  templateUrl: './games.html',
  styleUrl: './games.css',
})
export class GamesComponent{
  searchTerm = '';
  constructor() { }
  private gamesService = inject(GamesService);
  games = toSignal(
    this.gamesService.getGames().pipe(
      map((response) => response.games),
      catchError((error) => {
        console.error('Error fetching games:', error);
        return of([] as Game[]);
      })
    ),
    { initialValue: [] }
  );
}
