import { Routes } from '@angular/router';
import { GamesComponent } from './pages/games/games';
import { SearchComponent } from './pages/search/search';
import { LandingComponent } from './pages/landing/landing';
import { adminGuard, authGuard, guestGuard } from './guards/auth.guard';

export const routes: Routes = [
    { path: '', component: LandingComponent },
    { path: 'games', component: GamesComponent },
    { path: 'search', component: SearchComponent },
    // Lazy: pulls in Chart.js, which the landing/browse pages don't need.
    { path: 'games/:app_id', loadComponent: () => import('./pages/game-detail/game-detail').then((m) => m.GameDetailComponent) },

    { path: 'bundles', loadComponent: () => import('./pages/bundles/bundles').then((m) => m.BundlesComponent) },
    {
        path: 'bundles/:bundle_id',
        loadComponent: () => import('./pages/bundle-detail/bundle-detail').then((m) => m.BundleDetailComponent),
    },

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
        path: 'admin',
        canActivate: [adminGuard],
        loadComponent: () => import('./pages/admin/admin').then((m) => m.AdminComponent),
    },
    {
        path: 'submit',
        canActivate: [authGuard],
        loadComponent: () => import('./pages/submit/submit').then((m) => m.SubmitComponent),
    },
];
