import { Routes } from '@angular/router';
import { GamesComponent } from './pages/games/games';
import { GameDetailComponent } from './pages/game-detail/game-detail';

export const routes: Routes = [
    {
        path: 'games',
        component: GamesComponent,
    },
    {
        path: '',
        redirectTo: 'games',
        pathMatch: 'full',
    },
    {
        path: 'games/:app_id',
        component: GameDetailComponent
    }
];
