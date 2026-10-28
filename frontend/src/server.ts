import {
  AngularNodeAppEngine,
  createNodeRequestHandler,
  isMainModule,
  writeResponseToNodeResponse,
} from '@angular/ssr/node';
import express from 'express';
import { join } from 'node:path';

import { environment } from './environments/environment';
import { baseSecurityHeaders, inlineScriptSources, originOf, pageCsp } from './csp';

const browserDistFolder = join(import.meta.dirname, '../browser');

const app = express();
const angularApp = new AngularNodeAppEngine();

const apiOrigin = originOf(environment.apiUrl);
// CSP_REPORT_ONLY=true sends the policy as report-only: the browser logs what
// it would block instead of blocking it. Useful when trying out a change.
const cspHeader = process.env['CSP_REPORT_ONLY'] === 'true' ? 'Content-Security-Policy-Report-Only' : 'Content-Security-Policy';

app.disable('x-powered-by');

/**
 * Security headers on everything. HSTS only means something over HTTPS, so it
 * is sent when the request came in that way (directly or through a proxy).
 * The CSP set here allows no inline scripts at all; rendered pages replace it
 * with one that allows Angular's (see withPageCsp).
 */
app.use((req, res, next) => {
  const https = req.secure || req.get('x-forwarded-proto') === 'https';
  res.set(baseSecurityHeaders(https));
  res.set(cspHeader, pageCsp([], apiOrigin));
  next();
});

/**
 * Serve static files from /browser
 */
app.use(
  express.static(browserDistFolder, {
    maxAge: '1y',
    index: false,
    redirect: false,
  }),
);

/**
 * Handle all other requests by rendering the Angular application.
 */
app.use((req, res, next) => {
  angularApp
    .handle(req)
    .then((response) => (response ? withPageCsp(response) : null))
    .then((response) =>
      response ? writeResponseToNodeResponse(response, res) : next(),
    )
    .catch(next);
});

/** Adds the CSP to a rendered page, allowing the inline scripts Angular put in it. */
async function withPageCsp(response: Response): Promise<Response> {
  if (!response.headers.get('content-type')?.includes('text/html')) {
    return response;
  }
  const html = await response.text();
  const headers = new Headers(response.headers);
  headers.set(cspHeader, pageCsp(await inlineScriptSources(html), apiOrigin));
  headers.delete('content-length');
  return new Response(html, { status: response.status, statusText: response.statusText, headers });
}

/**
 * Start the server if this module is the main entry point, or it is ran via PM2.
 * The server listens on the port defined by the `PORT` environment variable, or defaults to 4000.
 */
if (isMainModule(import.meta.url) || process.env['pm_id']) {
  const port = process.env['PORT'] || 4000;
  app.listen(port, (error) => {
    if (error) {
      throw error;
    }

  });
}

export const reqHandler = createNodeRequestHandler(app);
