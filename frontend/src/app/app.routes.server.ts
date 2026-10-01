import { RenderMode, ServerRoute } from '@angular/ssr';

// Pages that depend on who's logged in are rendered in the browser only:
// the SSR server never has the user's session cookie, so it can't know.
const CLIENT_ONLY = ['login', 'register', 'feed', 'account', 'submit', 'admin'];

export const serverRoutes: ServerRoute[] = [
  ...CLIENT_ONLY.map((path) => ({ path, renderMode: RenderMode.Client }) as ServerRoute),
  {
    path: 'games/:app_id',
    renderMode: RenderMode.Server
  },
  {
    path: 'bundles/:bundle_id',
    renderMode: RenderMode.Server
  },
  {
    path: '**',
    renderMode: RenderMode.Prerender
  },
];
