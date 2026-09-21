import { Routes } from '@angular/router';
import { GamesComponent } from './pages/games/games';
import { GameDetailComponent } from './pages/game-detail/game-detail';
import { SearchComponent } from './pages/search/search';

export const routes: Routes = [
    {
        path: 'games',
        component: GamesComponent,
    },
    {
        path: 'search',
        component: SearchComponent,
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
