import { Routes } from '@angular/router';
import { GamesComponent } from './pages/games/games';
import { GameDetailComponent } from './pages/game-detail/game-detail';
import { SearchComponent } from './pages/search/search';
import { LandingComponent } from './pages/landing/landing';
import { authGuard, guestGuard } from './guards/auth.guard';

export const routes: Routes = [
    { path: '', component: LandingComponent },
    { path: 'games', component: GamesComponent },
    { path: 'search', component: SearchComponent },
    { path: 'games/:app_id', component: GameDetailComponent },

    // Account pages are lazy-loaded: most visitors never open them.
    {
        path: 'login',
        canActivate: [guestGuard],
        loadComponent: () => import('./pages/login/login').then((m) => m.LoginComponent),
    },
    {
        path: 'register',
        canActivate: [guestGuard],
        loadComponent: () => import('./pages/register/register').then((m) => m.RegisterComponent),
    },
    {
        path: 'feed',
        canActivate: [authGuard],
        loadComponent: () => import('./pages/feed/feed').then((m) => m.FeedComponent),
    },
    {
        path: 'account',
        canActivate: [authGuard],
        loadComponent: () => import('./pages/account/account').then((m) => m.AccountComponent),
    },
    {
        path: 'submit',
        canActivate: [authGuard],
        loadComponent: () => import('./pages/submit/submit').then((m) => m.SubmitComponent),
    },
];
