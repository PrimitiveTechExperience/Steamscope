import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { NEVER, Observable, of, throwError } from 'rxjs';

import { AccountComponent } from './account';
import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { GamesService } from '../../services/games';
import { PushError, PushService } from '../../services/push';
import { AlertChannels, Preferences } from '../../models/user';
import { fakeAuth, makeUser, textOf } from '../../../testing/factories';

function makePrefs(over: Partial<Preferences> = {}): Preferences {
  return {
    theme: 'dark', notify_price_drops: true, price_drop_threshold_percent: 10, preferred_genres: [],
    alert_email: false, alert_discord: false, discord_webhook_url: '', alert_push: false, ...over,
  };
}

const ALL: AlertChannels = { email: true, discord: true, push: true, vapid_public_key: 'vapid-key' };
const HOOK = 'https://discord.com/api/webhooks/123456789012345678/abcdefghijklmnopqrstuvwxyz';

interface Opts {
  prefs?: Preferences;
  channels?: AlertChannels | Error;
  push?: Partial<{ supported: boolean; blocked: boolean; enable: unknown; current: unknown; disable: unknown }>;
  save?: Observable<Preferences>;
  test?: Observable<void>;
  saveSub?: Observable<void>;
}

async function render(opts: Opts = {}) {
  const account = {
    getPreferences: vi.fn().mockReturnValue(of(opts.prefs ?? makePrefs())),
    updatePreferences: vi.fn().mockImplementation((p: Preferences) => opts.save ?? of(p)),
    getAlertChannels: vi.fn().mockReturnValue(opts.channels instanceof Error ? throwError(() => opts.channels) : of(opts.channels ?? ALL)),
    savePushSubscription: vi.fn().mockReturnValue(opts.saveSub ?? of(undefined)),
    deletePushSubscription: vi.fn().mockReturnValue(of(undefined)),
    sendTestAlert: vi.fn().mockReturnValue(opts.test ?? of(undefined)),
    unlinkSteam: vi.fn(),
  };
  const push = {
    supported: true,
    blocked: false,
    enable: vi.fn().mockResolvedValue({ endpoint: 'https://push.example/e1', keys: { p256dh: 'k', auth: 'a' } }),
    current: vi.fn().mockResolvedValue(null),
    disable: vi.fn().mockResolvedValue('https://push.example/e1'),
    ...opts.push,
  };
  TestBed.configureTestingModule({
    imports: [AccountComponent],
    providers: [
      provideRouter([]),
      { provide: AuthService, useValue: fakeAuth(makeUser({ email: 'ann@example.com' })) },
      { provide: AccountService, useValue: account },
      { provide: GamesService, useValue: { getFilterOptions: () => of({ genres: [], tags: [], developers: [], publishers: [], languages: [] }) } },
      { provide: PushService, useValue: push },
    ],
  });
  const fixture = TestBed.createComponent(AccountComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const q = (s: string) => el.querySelector(s);
  const box = (s: string) => q(s) as HTMLInputElement;
  const tick = async (s: string) => {
    box(s).click();
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const click = async (s: string) => {
    (q(s) as HTMLElement).click();
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const type = (s: string, value: string) => {
    const input = box(s);
    input.value = value;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  };
  const save = async () => {
    const button = Array.from(el.querySelectorAll('button')).find((b) => textOf(b).includes('Save preferences'))!;
    button.click();
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const savedPrefs = () => account.updatePreferences.mock.calls.at(-1)![0] as Preferences;
  return { fixture, el, account, push, q, box, tick, click, type, save, savedPrefs };
}

describe('AccountComponent target-price alerts', () => {
  it('offers email, Discord and browser notification, all off to begin with', async () => {
    const { q, box } = await render();
    expect(textOf(q('.alerts legend'))).toBe('Target-price alerts');
    expect(box('.email-box').checked).toBe(false);
    expect(box('.discord-box').checked).toBe(false);
    expect(box('.push-box').checked).toBe(false);
    expect(q('.discord-url')).toBeNull(); // nothing to type until Discord is ticked
    expect(q('.test-email')).toBeNull();
    expect(q('.test-push')).toBeNull();
  });

  it('shows the saved settings', async () => {
    const { q, box } = await render({ prefs: makePrefs({ alert_email: true, alert_discord: true, discord_webhook_url: HOOK, alert_push: true }), push: { current: vi.fn().mockResolvedValue({ endpoint: 'x' }) } });
    expect(box('.email-box').checked).toBe(true);
    expect(box('.discord-box').checked).toBe(true);
    expect(box('.push-box').checked).toBe(true);
    expect(box('.discord-url').value).toBe(HOOK);
    expect(q('.push-elsewhere')).toBeNull(); // this browser is subscribed
  });

  describe('email', () => {
    it('says which address it goes to', async () => {
      const { q } = await render();
      expect(textOf(q('.alert-email'))).toContain('ann@example.com');
    });

    it('is saved when ticked and Save is pressed', async () => {
      const { tick, save, savedPrefs, account } = await render();
      await tick('.email-box');
      expect(account.updatePreferences).not.toHaveBeenCalled(); // not until Save, like the other preferences
      await save();
      expect(savedPrefs().alert_email).toBe(true);
    });

    it('cannot be ticked, and says why, when the server has no mail settings', async () => {
      const { q, box } = await render({ channels: { ...ALL, email: false } });
      expect(box('.email-box').disabled).toBe(true);
      expect(textOf(q('.email-unavailable'))).toBe('Not set up on this server.');
    });

    it('can still be unticked if it was on before the server lost its mail settings', async () => {
      const { box } = await render({ channels: { ...ALL, email: false }, prefs: makePrefs({ alert_email: true }) });
      expect(box('.email-box').disabled).toBe(false);
    });

    it('offers a test email once it is on, and reports the result', async () => {
      const { q, tick, click, account } = await render();
      await tick('.email-box');
      expect(textOf(q('.test-email'))).toBe('Send a test email');
      await click('.test-email');
      expect(account.sendTestAlert).toHaveBeenCalledWith('email', undefined);
      expect(textOf(q('.alert-email .test-result'))).toBe('Sent. Check for it now.');
    });

    it('shows why a test email failed', async () => {
      const err = new HttpErrorResponse({ status: 502, error: { error: 'The mail server could not send the email.' } });
      const { q, tick, click } = await render({ test: throwError(() => err) });
      await tick('.email-box');
      await click('.test-email');
      expect(textOf(q('.alert-email .test-result'))).toBe('The mail server could not send the email.');
    });
  });

  describe('Discord', () => {
    it('asks for a webhook URL when ticked, with how to get one', async () => {
      const { q, tick } = await render();
      await tick('.discord-box');
      expect(q('.discord-url')).not.toBeNull();
      expect(textOf(q('.alert-discord'))).toContain('Copy Webhook URL');
    });

    it('saves the URL with the box', async () => {
      const { tick, type, save, savedPrefs } = await render();
      await tick('.discord-box');
      type('.discord-url', HOOK);
      await save();
      expect(savedPrefs()).toMatchObject({ alert_discord: true, discord_webhook_url: HOOK });
    });

    it('keeps the URL visible when the box is unticked, so it is not lost', async () => {
      const { q, box } = await render({ prefs: makePrefs({ alert_discord: false, discord_webhook_url: HOOK }) });
      expect(box('.discord-url').value).toBe(HOOK);
      expect(q('.discord-box')).not.toBeNull();
    });

    it('cannot send a test until there is a URL', async () => {
      const { q, tick, type } = await render();
      await tick('.discord-box');
      expect((q('.test-discord') as HTMLButtonElement).disabled).toBe(true);
      type('.discord-url', '   ');
      expect((q('.test-discord') as HTMLButtonElement).disabled).toBe(true);
      type('.discord-url', HOOK);
      expect((q('.test-discord') as HTMLButtonElement).disabled).toBe(false);
    });

    it('tests the URL as typed, even before it is saved', async () => {
      const { tick, type, click, account, q } = await render();
      await tick('.discord-box');
      type('.discord-url', ' ' + HOOK + ' ');
      await click('.test-discord');
      expect(account.sendTestAlert).toHaveBeenCalledWith('discord', HOOK);
      expect(textOf(q('.alert-discord .test-result'))).toBe('Sent. Check for it now.');
    });

    it('shows what Discord said when the webhook is rejected', async () => {
      const err = new HttpErrorResponse({ status: 502, error: { error: 'Discord rejected the webhook (it answered 404). Check the URL.' } });
      const { q, tick, type, click } = await render({ test: throwError(() => err) });
      await tick('.discord-box');
      type('.discord-url', HOOK);
      await click('.test-discord');
      expect(textOf(q('.alert-discord .test-result'))).toContain('Discord rejected the webhook');
    });

    it('disables the test button while a test is being sent', async () => {
      const { q, tick, type, click } = await render({ test: NEVER });
      await tick('.discord-box');
      type('.discord-url', HOOK);
      await click('.test-discord');
      expect(textOf(q('.test-discord'))).toBe('Sending...');
      expect((q('.test-discord') as HTMLButtonElement).disabled).toBe(true);
    });
  });

  describe('browser notifications', () => {
    it('asks the browser for permission, subscribes, tells the server and ticks the box', async () => {
      const { tick, box, push, account, q } = await render();
      await tick('.push-box');
      expect(push.enable).toHaveBeenCalledWith('vapid-key');
      expect(account.savePushSubscription).toHaveBeenCalledWith({ endpoint: 'https://push.example/e1', keys: { p256dh: 'k', auth: 'a' } });
      expect(box('.push-box').checked).toBe(true);
      expect(q('.push-message')).toBeNull();
    });

    it('is saved with the other preferences', async () => {
      const { tick, save, savedPrefs } = await render();
      await tick('.push-box');
      await save();
      expect(savedPrefs().alert_push).toBe(true);
    });

    it('stays off, with the reason, when the user blocks the permission prompt', async () => {
      const denied = vi.fn().mockRejectedValue(new PushError('Notifications are blocked for this site. Allow them in your browser settings, then try again.'));
      const { tick, box, q, account } = await render({ push: { enable: denied } });
      await tick('.push-box');
      expect(box('.push-box').checked).toBe(false);
      expect(textOf(q('.push-message'))).toContain('Notifications are blocked');
      expect(account.savePushSubscription).not.toHaveBeenCalled();
    });

    it('stays off when the server cannot save the subscription', async () => {
      const err = new HttpErrorResponse({ status: 503, error: { error: 'push notifications are not set up on this server' } });
      const { tick, box, q } = await render({ saveSub: throwError(() => err) });
      await tick('.push-box');
      expect(box('.push-box').checked).toBe(false);
      expect(textOf(q('.push-message'))).toBe('Push notifications are not set up on this server');
    });

    it('unsubscribes this browser and tells the server when unticked', async () => {
      const { tick, box, push, account } = await render({ prefs: makePrefs({ alert_push: true }), push: { current: vi.fn().mockResolvedValue({ endpoint: 'x' }) } });
      await tick('.push-box');
      expect(push.disable).toHaveBeenCalled();
      expect(account.deletePushSubscription).toHaveBeenCalledWith('https://push.example/e1');
      expect(box('.push-box').checked).toBe(false);
    });

    it('does not call the server when this browser had no subscription to remove', async () => {
      const { tick, account } = await render({ prefs: makePrefs({ alert_push: true }), push: { disable: vi.fn().mockResolvedValue(null) } });
      await tick('.push-box');
      expect(account.deletePushSubscription).not.toHaveBeenCalled();
    });

    it('offers to subscribe this browser when the setting is on but this browser is not subscribed', async () => {
      const { q, click, push, account } = await render({ prefs: makePrefs({ alert_push: true }) });
      expect(textOf(q('.push-elsewhere'))).toContain("This browser isn't subscribed yet.");
      await click('.enable-here');
      expect(push.enable).toHaveBeenCalled();
      expect(account.savePushSubscription).toHaveBeenCalled();
      expect(q('.push-elsewhere')).toBeNull();
    });

    it.each([
      ['the server has no push keys', { channels: { ...ALL, push: false } }, 'Not set up on this server.'],
      ['the browser has no push support', { push: { supported: false } }, "This browser doesn't support push notifications."],
      ['the user blocked notifications in the browser', { push: { blocked: true } }, 'Notifications are blocked for this site in your browser settings.'],
    ] as const)('cannot be ticked when %s, and says why', async (_name, opts, reason) => {
      const { q, box } = await render(opts as Opts);
      expect(box('.push-box').disabled).toBe(true);
      expect(textOf(q('.push-unavailable'))).toBe(reason);
    });

    it('sends a test notification once on', async () => {
      const { q, click, account } = await render({ prefs: makePrefs({ alert_push: true }), push: { current: vi.fn().mockResolvedValue({ endpoint: 'x' }) } });
      await click('.test-push');
      expect(account.sendTestAlert).toHaveBeenCalledWith('push', undefined);
      expect(textOf(q('.alert-push .test-result'))).toBe('Sent. Check for it now.');
    });
  });

  describe('saving', () => {
    it('shows the server\'s reason when it refuses a setting', async () => {
      const err = new HttpErrorResponse({ status: 400, error: { error: 'that is not a Discord webhook URL' } });
      const { tick, type, save, el } = await render({ save: throwError(() => err) });
      await tick('.discord-box');
      type('.discord-url', 'https://example.com/x');
      await save();
      expect(textOf(el)).toContain('That is not a Discord webhook URL');
    });

    it('keeps the existing preferences when only an alert is changed', async () => {
      const { tick, save, savedPrefs } = await render({ prefs: makePrefs({ theme: 'light', price_drop_threshold_percent: 25 }) });
      await tick('.email-box');
      await save();
      expect(savedPrefs()).toMatchObject({ theme: 'light', price_drop_threshold_percent: 25, alert_email: true });
    });
  });

  it('offers only what needs nothing from the server when the channel list cannot be loaded', async () => {
    const { box, q } = await render({ channels: new Error('down') });
    expect(box('.discord-box').disabled).toBe(false);
    expect(box('.email-box').disabled).toBe(true);
    expect(textOf(q('.push-unavailable'))).toBe('Not set up on this server.');
  });
});
