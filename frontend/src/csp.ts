/**
 * Security headers and the Content-Security-Policy for pages served by the
 * SSR server (src/server.ts). Kept apart from the server so it can be unit
 * tested.
 *
 * Scripts: only files from this site run, plus the two inline scripts
 * Angular writes into server-rendered pages for event replay. Those are
 * allowed by their SHA-256, worked out for each page, so nothing else inline
 * can run, including anything injected into the page. The stylesheet loader
 * Angular adds (onload="this.media='all'") is allowed the same way.
 *
 * Styles allow 'unsafe-inline': Angular puts component styles in <style>
 * tags and Chart.js sets style attributes, and neither has a nonce.
 */

/** Hosts Steam serves game art, screenshots and avatars from. */
export const STEAM_IMAGE_HOSTS = [
  'https://*.steamstatic.com',
  'https://*.steampowered.com',
  'https://*.steamusercontent.com',
  'https://*.akamaihd.net',
];

/** The stylesheet loader Angular adds when it inlines critical CSS. */
export const STYLESHEET_ONLOAD = "this.media='all'";

/** What Angular's event-replay bootstrap looks like: app ID, then two lists of event names. */
const EVENT_LIST = String.raw`\[(?:"[a-zA-Z:]+"(?:,"[a-zA-Z:]+")*)?\]`;
const JSACTION_BOOTSTRAP = new RegExp(String.raw`^window\.__jsaction_bootstrap\(document\.body,"[\w-]+",${EVENT_LIST},${EVENT_LIST}\);$`);

const SCRIPT_TAG = /<script\b([^>]*)>([\s\S]*?)<\/script\s*>/gi;

async function sha256Source(text: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text));
  let binary = '';
  for (const byte of new Uint8Array(digest)) binary += String.fromCharCode(byte);
  return `'sha256-${btoa(binary)}'`;
}

/**
 * The CSP sources for the inline scripts in a page that are Angular's own.
 * Any other inline script gets no hash, so the browser refuses to run it.
 */
export async function inlineScriptSources(html: string): Promise<string[]> {
  const sources: string[] = [];
  let sawContract = false;
  for (const [, attrs, body] of html.matchAll(SCRIPT_TAG)) {
    if (/\ssrc\s*=/i.test(' ' + attrs) || /type\s*=\s*["']?application\/json/i.test(attrs)) continue;
    const isContract = /\bid\s*=\s*["']?ng-event-dispatch-contract\b/i.test(attrs);
    if ((isContract && !sawContract) || JSACTION_BOOTSTRAP.test(body)) {
      sawContract ||= isContract;
      sources.push(await sha256Source(body));
    }
  }
  if (html.includes(`onload="${STYLESHEET_ONLOAD}"`)) {
    sources.push("'unsafe-hashes'", await sha256Source(STYLESHEET_ONLOAD));
  }
  return [...new Set(sources)];
}

/** The page CSP. apiOrigin is where the browser calls the API. */
export function pageCsp(scriptSources: string[], apiOrigin: string): string {
  const connect = ["'self'", apiOrigin].filter((v, i, all) => v && all.indexOf(v) === i);
  return [
    "default-src 'self'",
    `script-src 'self' ${scriptSources.join(' ')}`.trim(),
    "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
    "font-src 'self' https://fonts.gstatic.com data:",
    `img-src 'self' data: ${STEAM_IMAGE_HOSTS.join(' ')}`,
    `connect-src ${connect.join(' ')}`,
    "worker-src 'self'",
    "manifest-src 'self'",
    "object-src 'none'",
    "frame-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ].join('; ');
}

/** The origin part of the API URL, e.g. https://api.example.com. */
export function originOf(url: string): string {
  try {
    return new URL(url).origin;
  } catch {
    return '';
  }
}

/** Headers for every response, pages and files alike. */
export function baseSecurityHeaders(https: boolean): Record<string, string> {
  const headers: Record<string, string> = {
    'X-Content-Type-Options': 'nosniff',
    'X-Frame-Options': 'DENY',
    'Referrer-Policy': 'strict-origin-when-cross-origin',
    'Cross-Origin-Opener-Policy': 'same-origin',
    'Permissions-Policy': 'camera=(), microphone=(), geolocation=(), payment=()',
  };
  if (https) headers['Strict-Transport-Security'] = 'max-age=31536000';
  return headers;
}
