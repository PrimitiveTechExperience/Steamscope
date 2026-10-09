import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// The service worker (public/push-sw.js) runs in a browser worker, so it is
// loaded here against a stand-in for its global scope and driven with the same
// events the browser sends.

const SOURCE = readFileSync(resolve(process.cwd(), 'public/push-sw.js'), 'utf-8');
const ORIGIN = 'https://steamscope.example';

interface WindowStub {
  navigate: ReturnType<typeof vi.fn>;
  focus: ReturnType<typeof vi.fn>;
}

function load(windows: WindowStub[] = []) {
  const listeners: Record<string, (event: any) => void> = {};
  const scope = {
    location: { origin: ORIGIN },
    skipWaiting: vi.fn(),
    addEventListener: (type: string, fn: (event: any) => void) => (listeners[type] = fn),
    registration: { showNotification: vi.fn().mockResolvedValue(undefined) },
    clients: {
      claim: vi.fn().mockResolvedValue(undefined),
      matchAll: vi.fn().mockResolvedValue(windows),
      openWindow: vi.fn().mockResolvedValue(undefined),
    },
  };
  new Function('self', SOURCE)(scope);

  /** An event whose waitUntil promise can be awaited. */
  const event = <T extends object>(extra: T) => {
    const waits: Promise<unknown>[] = [];
    return { ...extra, waitUntil: (p: Promise<unknown>) => waits.push(p), done: () => Promise.all(waits) };
  };
  return { scope, listeners, event };
}

function pushEvent(env: ReturnType<typeof load>, data: { json?: () => unknown; text?: () => string } | null) {
  const e = env.event({ data });
  env.listeners['push'](e);
  return e;
}

function clickEvent(env: ReturnType<typeof load>, data: unknown) {
  const close = vi.fn();
  const e = env.event({ notification: { close, data } });
  env.listeners['notificationclick'](e);
  return { e, close };
}

describe('push service worker', () => {
  it('registers handlers for the four events it uses', () => {
    const { listeners } = load();
    expect(Object.keys(listeners).sort()).toEqual(['activate', 'install', 'notificationclick', 'push']);
  });

  it('takes over straight away instead of waiting for old tabs to close', async () => {
    const env = load();
    env.listeners['install']({});
    expect(env.scope.skipWaiting).toHaveBeenCalled();
    const e = env.event({});
    env.listeners['activate'](e);
    await e.done();
    expect(env.scope.clients.claim).toHaveBeenCalled();
  });

  describe('showing an alert', () => {
    it('shows the title, text and link the server sent', async () => {
      const env = load();
      const e = pushEvent(env, {
        json: () => ({ title: 'Target price reached', body: 'Hades is now $9.99', url: ORIGIN + '/games/1145360', tag: 'target-price' }),
      });
      await e.done();
      expect(env.scope.registration.showNotification).toHaveBeenCalledWith('Target price reached', {
        body: 'Hades is now $9.99',
        tag: 'target-price',
        icon: '/favicon.ico',
        data: { url: ORIGIN + '/games/1145360' },
      });
    });

    it('keeps the browser alive until the notification is shown', async () => {
      const env = load();
      let shown = false;
      env.scope.registration.showNotification.mockImplementation(() => new Promise<void>((r) => setTimeout(() => ((shown = true), r()), 10)));
      const e = pushEvent(env, { json: () => ({ title: 't' }) });
      await e.done();
      expect(shown).toBe(true);
    });

    it('fills in sensible defaults for a message with only some fields', async () => {
      const env = load();
      await pushEvent(env, { json: () => ({ body: 'only a body' }) }).done();
      expect(env.scope.registration.showNotification).toHaveBeenCalledWith('Steamscope', {
        body: 'only a body', tag: 'steamscope', icon: '/favicon.ico', data: { url: '/' },
      });
    });

    it('shows a plain-text message when the body is not JSON', async () => {
      const env = load();
      await pushEvent(env, {
        json: () => {
          throw new SyntaxError('not json');
        },
        text: () => 'plain words',
      }).done();
      expect(env.scope.registration.showNotification).toHaveBeenCalledWith('Steamscope', expect.objectContaining({ body: 'plain words' }));
    });

    it('still shows something when the message has no data at all', async () => {
      const env = load();
      await pushEvent(env, null).done();
      expect(env.scope.registration.showNotification).toHaveBeenCalledWith('Steamscope', expect.objectContaining({ body: '', data: { url: '/' } }));
    });
  });

  describe('clicking an alert', () => {
    const win = (): WindowStub => ({ navigate: vi.fn(), focus: vi.fn().mockResolvedValue(undefined) });

    it('closes the notification', async () => {
      const env = load();
      const { e, close } = clickEvent(env, { url: ORIGIN + '/games/1' });
      await e.done();
      expect(close).toHaveBeenCalled();
    });

    it('takes an open tab to the game and brings it forward', async () => {
      const w = win();
      const env = load([w]);
      const { e } = clickEvent(env, { url: ORIGIN + '/games/1145360' });
      await e.done();
      expect(w.navigate).toHaveBeenCalledWith(ORIGIN + '/games/1145360');
      expect(w.focus).toHaveBeenCalled();
      expect(env.scope.clients.openWindow).not.toHaveBeenCalled();
    });

    it('opens a new tab when none is open', async () => {
      const env = load([]);
      const { e } = clickEvent(env, { url: ORIGIN + '/games/1145360' });
      await e.done();
      expect(env.scope.clients.openWindow).toHaveBeenCalledWith(ORIGIN + '/games/1145360');
    });

    it('looks at all of the site\'s tabs, including ones this worker does not control yet', async () => {
      const env = load();
      await clickEvent(env, { url: ORIGIN + '/' }).e.done();
      expect(env.scope.clients.matchAll).toHaveBeenCalledWith({ type: 'window', includeUncontrolled: true });
    });

    it('resolves a relative link against the site', async () => {
      const env = load([]);
      await clickEvent(env, { url: '/account' }).e.done();
      expect(env.scope.clients.openWindow).toHaveBeenCalledWith(ORIGIN + '/account');
    });

    it('goes to the home page when the notification carries no link', async () => {
      const env = load([]);
      await clickEvent(env, undefined).e.done();
      expect(env.scope.clients.openWindow).toHaveBeenCalledWith(ORIGIN + '/');
    });

    it.each([
      ['another site', 'https://evil.example/phish'],
      ['a lookalike host', ORIGIN + '.evil.example/x'],
      ['a different port', 'https://steamscope.example:8443/x'],
      ['a javascript: link', 'javascript:alert(1)'],
      ['a data: link', 'data:text/html,<script>alert(1)</script>'],
      ['a scheme-relative link to another host', '//evil.example/x'],
    ])('never opens %s, whatever the message said', async (_name, url) => {
      const env = load([]);
      await clickEvent(env, { url }).e.done();
      expect(env.scope.clients.openWindow).toHaveBeenCalledWith(ORIGIN + '/');
    });

    it('does not navigate an open tab to a link outside the site either', async () => {
      const w = win();
      const env = load([w]);
      await clickEvent(env, { url: 'https://evil.example/phish' }).e.done();
      expect(w.navigate).toHaveBeenCalledWith(ORIGIN + '/');
    });
  });
});
