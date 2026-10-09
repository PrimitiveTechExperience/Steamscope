import { TestBed } from '@angular/core/testing';

import { PUSH_WORKER_URL, PushError, PushService, urlBase64ToBytes } from './push';

describe('urlBase64ToBytes', () => {
  it('decodes the url-safe base64 a VAPID key is written in', () => {
    // "hello?>" encodes to "aGVsbG8_Pg" in url-safe base64 with no padding.
    expect(Array.from(urlBase64ToBytes('aGVsbG8_Pg'))).toEqual(Array.from(new TextEncoder().encode('hello?>')));
  });

  it('handles - and _ and missing padding', () => {
    expect(Array.from(urlBase64ToBytes('-_8'))).toEqual([251, 255]);
  });

  it('turns a real-length key into 65 bytes', () => {
    const key = 'BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkvMeAtA3LFgDzkrxZJjSgSnfckjBJuBkr3qBUYIHBQFLXYp5Nksh8U';
    expect(urlBase64ToBytes(key).length).toBe(65);
  });
});

interface FakeSubscription {
  endpoint: string;
  toJSON: () => { keys: { p256dh: string; auth: string } };
  unsubscribe: ReturnType<typeof vi.fn>;
}

function fakeSubscription(endpoint = 'https://push.example/1'): FakeSubscription {
  return { endpoint, toJSON: () => ({ keys: { p256dh: 'P', auth: 'A' } }), unsubscribe: vi.fn().mockResolvedValue(true) };
}

/** Puts a working (or deliberately broken) push stack on the global objects. */
function installBrowser(opts: { permission?: NotificationPermission; request?: NotificationPermission; existing?: FakeSubscription | null; subscribeFails?: boolean } = {}) {
  let existing = opts.existing ?? null;
  const subscribe = vi.fn().mockImplementation(async () => {
    if (opts.subscribeFails) throw new Error('nope');
    existing = fakeSubscription();
    return existing;
  });
  const registration = { pushManager: { getSubscription: vi.fn().mockImplementation(async () => existing), subscribe } };
  const register = vi.fn().mockResolvedValue(registration);
  Object.defineProperty(navigator, 'serviceWorker', {
    configurable: true,
    value: { register, ready: Promise.resolve(registration), getRegistration: vi.fn().mockResolvedValue(registration) },
  });
  vi.stubGlobal('PushManager', class {});
  const requestPermission = vi.fn().mockResolvedValue(opts.request ?? 'granted');
  vi.stubGlobal('Notification', { permission: opts.permission ?? 'default', requestPermission });
  return { register, subscribe, requestPermission, registration, current: () => existing };
}

afterEach(() => {
  vi.unstubAllGlobals();
  // @ts-expect-error removing the stub added for the test
  delete navigator.serviceWorker;
});

describe('PushService', () => {
  const service = () => TestBed.inject(PushService);

  it('knows when the browser can do push', () => {
    installBrowser();
    expect(service().supported).toBe(true);
  });

  it('knows when it cannot', () => {
    // jsdom has no service worker, PushManager or Notification
    expect(service().supported).toBe(false);
    expect(service().blocked).toBe(false);
  });

  it('knows when the user has blocked notifications', () => {
    installBrowser({ permission: 'denied' });
    expect(service().blocked).toBe(true);
  });

  describe('enable', () => {
    it('asks permission, registers the worker, subscribes with the server key, and returns what the server needs', async () => {
      const b = installBrowser();
      const result = await service().enable('aGVsbG8_Pg');
      expect(b.requestPermission).toHaveBeenCalledTimes(1);
      expect(b.register).toHaveBeenCalledWith(PUSH_WORKER_URL);
      const options = b.subscribe.mock.calls[0][0];
      expect(options.userVisibleOnly).toBe(true);
      expect(Array.from(options.applicationServerKey as Uint8Array)).toEqual(Array.from(urlBase64ToBytes('aGVsbG8_Pg')));
      expect(result).toEqual({ endpoint: 'https://push.example/1', keys: { p256dh: 'P', auth: 'A' } });
    });

    it('does not ask again once permission was already given', async () => {
      const b = installBrowser({ permission: 'granted' });
      await service().enable('aGVsbG8_Pg');
      expect(b.requestPermission).not.toHaveBeenCalled();
    });

    it('reuses the browser subscription it already has instead of making another', async () => {
      const b = installBrowser({ permission: 'granted', existing: fakeSubscription('https://push.example/already') });
      const result = await service().enable('aGVsbG8_Pg');
      expect(b.subscribe).not.toHaveBeenCalled();
      expect(result.endpoint).toBe('https://push.example/already');
    });

    it('explains a refusal and subscribes to nothing', async () => {
      const b = installBrowser({ request: 'denied' });
      await expect(service().enable('aGVsbG8_Pg')).rejects.toThrow(/blocked/);
      expect(b.subscribe).not.toHaveBeenCalled();
      expect(b.register).not.toHaveBeenCalled();
    });

    it('treats dismissing the prompt as a refusal too', async () => {
      installBrowser({ request: 'default' });
      await expect(service().enable('aGVsbG8_Pg')).rejects.toBeInstanceOf(PushError);
    });

    it('says so when the browser cannot subscribe', async () => {
      installBrowser({ permission: 'granted', subscribeFails: true });
      await expect(service().enable('aGVsbG8_Pg')).rejects.toThrow("Couldn't subscribe this browser");
    });

    it('refuses without a server key', async () => {
      installBrowser();
      await expect(service().enable('')).rejects.toThrow('not set up on this server');
    });

    it('refuses where push is unsupported', async () => {
      await expect(service().enable('aGVsbG8_Pg')).rejects.toThrow("doesn't support push");
    });
  });

  describe('current and disable', () => {
    it('reports no subscription when there is none, or no support', async () => {
      expect(await service().current()).toBeNull();
      installBrowser();
      expect(await service().current()).toBeNull();
    });

    it('reports the existing subscription', async () => {
      installBrowser({ existing: fakeSubscription('https://push.example/here') });
      expect((await service().current())?.endpoint).toBe('https://push.example/here');
    });

    it('unsubscribes and returns the endpoint so the server can forget it', async () => {
      const sub = fakeSubscription('https://push.example/bye');
      installBrowser({ existing: sub });
      expect(await service().disable()).toBe('https://push.example/bye');
      expect(sub.unsubscribe).toHaveBeenCalled();
    });

    it('returns null when there was nothing to unsubscribe', async () => {
      installBrowser();
      expect(await service().disable()).toBeNull();
    });
  });
});
