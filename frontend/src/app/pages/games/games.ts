import { Component, computed, inject} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { BehaviorSubject, catchError, map, of, switchMap } from 'rxjs';

import {GamesService} from '../../services/games';
import {Game} from '../../models/game';
import { GameCardComponent } from '../../components/game-card/game-card';
import { FormsModule } from '@angular/forms';
import { withLoading } from '../../utils/with-loading';

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
  private searchTrigger = new BehaviorSubject<string>('');

  private gamesState = toSignal(
    this.searchTrigger.pipe(
      switchMap((search) =>
        withLoading(
          this.gamesService.getGames({ search }).pipe(
            map((response) => response.games),
            catchError((error) => {
              console.error('Error fetching games:', error);
              return of([] as Game[]);
            })
          ),
          [] as Game[]
        )
      )
    ),
    { initialValue: { data: [] as Game[], loading: true } }
  );

  protected games = computed(() => this.gamesState().data);
  protected gamesLoading = computed(() => this.gamesState().loading);

  search() {
    this.searchTrigger.next(this.searchTerm);
  }
}
