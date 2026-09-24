import { Routes } from '@angular/router';
import { GamesComponent } from './pages/games/games';
import { GameDetailComponent } from './pages/game-detail/game-detail';
import { SearchComponent } from './pages/search/search';
import { LandingComponent } from './pages/landing/landing';

export const routes: Routes = [
    {
        path: '',
        component: LandingComponent,
    },
    {
        path: 'games',
        component: GamesComponent,
    },
    {
        path: 'search',
        component: SearchComponent,
    },
    {
        path: 'games/:app_id',
        component: GameDetailComponent
    }
];
