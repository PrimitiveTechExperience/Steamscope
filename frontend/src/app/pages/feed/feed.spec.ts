import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { HttpErrorResponse } from '@angular/common/http';

import { FeedComponent } from './feed';
import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { AppNotification, Feed, SteamProfile } from '../../models/user';
import { fakeAuth, makeUser, stubIntersectionObserver, textOf } from '../../../testing/factories';

const EMPTY: Feed = { watchlist: [], deals: [], suggestions: [] };

function profile(over: Partial<SteamProfile> = {}): SteamProfile {
  return {
    steam_id: '1', persona_name: 'GamerOne', avatar_url: 'https://avatars.example/a.jpg', profile_url: 'https://steamcommunity.com/id/gamerone',
    status: 'Online', currently_playing: '',
    recently_played: [
      { app_id: 1, name: 'Tracked Game', icon_url: 'https://i.example/1.jpg', playtime_2weeks: 150, playtime_forever: 900, track_status: 'tracked' },
      { app_id: 2, name: 'New Game', icon_url: '', playtime_2weeks: 30, playtime_forever: 60, track_status: '' },
      { app_id: 3, name: 'Waiting Game', icon_url: '', playtime_2weeks: 60, playtime_forever: 60, track_status: 'awaiting_approval' },
      { app_id: 4, name: 'Declined Game', icon_url: '', playtime_2weeks: 60, playtime_forever: 60, track_status: 'rejected' },
      { app_id: 5, name: 'Broken Game', icon_url: '', playtime_2weeks: 60, playtime_forever: 60, track_status: 'failed' },
    ],
    ...over,
  };
}

function notification(over: Partial<AppNotification> = {}): AppNotification {
  return { notification_id: 1, app_id: null, kind: 'price_drop', message: 'Hades dropped to $10', read_at: null, created_at: '2026-10-01T12:00:00Z', ...over };
}

function render(opts: { profile?: SteamProfile | null; profileError?: number; notifications?: AppNotification[]; submitGame?: unknown } = {}) {
  stubIntersectionObserver();
  const notifications = signal<AppNotification[]>(opts.notifications ?? []);
  const account = {
    notifications,
    unreadCount: () => notifications().filter((n) => !n.read_at).length,
    loadNotifications: vi.fn(),
    markRead: vi.fn().mockReturnValue(of(undefined)),
    markAllRead: vi.fn().mockReturnValue(of(undefined)),
    getFeed: vi.fn().mockReturnValue(of(EMPTY)),
    getSteamProfile: vi.fn().mockReturnValue(
      opts.profileError ? throwError(() => new HttpErrorResponse({ status: opts.profileError })) : of({ profile: opts.profile === undefined ? null : opts.profile })
    ),
    submitGame: opts.submitGame ?? vi.fn().mockReturnValue(of({ kind: 'app', id: 2, status: 'awaiting_approval' })),
    importWishlist: vi.fn(),
  };
  TestBed.configureTestingModule({
    imports: [FeedComponent],
    providers: [provideRouter([]), { provide: AuthService, useValue: fakeAuth(makeUser()) }, { provide: AccountService, useValue: account }],
  });
  const fixture = TestBed.createComponent(FeedComponent);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const tiles = () => Array.from(el.querySelectorAll('ul.grid li'));
  const tile = (name: string) => tiles().find((t) => textOf(t).includes(name))!;
  return { fixture, el, account, tile };
}

describe('FeedComponent: Steam profile', () => {
  it('greets the user by name', () => {
    expect(textOf(render().el.querySelector('h1'))).toMatch(/^(Up late|Good morning|Good afternoon|Good evening), alice ?\.$/);
  });

  it('shows the persona, status and hours played in the last two weeks', () => {
    const { el, tile } = render({ profile: profile() });
    expect(textOf(el)).toContain('GamerOne');
    expect(textOf(el)).toContain('Online');
    expect(el.querySelector('img[alt="GamerOne"]')?.getAttribute('src')).toBe('https://avatars.example/a.jpg');
    expect(textOf(tile('Tracked Game'))).toContain('2.5h past 2 weeks');
    expect(textOf(tile('New Game'))).toContain('0.5h past 2 weeks');
  });

  it('highlights what the user is currently playing', () => {
    const { el } = render({ profile: profile({ currently_playing: 'Hades', status: 'In-Game' }) });
    expect(textOf(el)).toContain('Playing Hades');
  });

  it('shows a different control for each tracking state', () => {
    const { tile } = render({ profile: profile() });
    const track = tile('New Game').querySelector('button');
    expect(textOf(track)).toBe('Track price');
    expect(textOf(tile('Broken Game').querySelector('button'))).toBe('Track price'); // failed ones can be retried

    expect(tile('Tracked Game').querySelector('button')).toBeNull();
    expect(tile('Tracked Game').querySelector('a')?.getAttribute('href')).toBe('/games/1');
    expect(textOf(tile('Tracked Game').querySelector('a'))).toBe('Tracked');

    expect(tile('Waiting Game').querySelector('button')).toBeNull();
    expect(textOf(tile('Waiting Game'))).toContain('Awaiting approval');
    expect(textOf(tile('Declined Game'))).toContain('Not approved');
  });

  it('requests tracking with the store URL and then shows the status', () => {
    const { fixture, account, tile } = render({ profile: profile() });
    (tile('New Game').querySelector('button') as HTMLButtonElement).click();
    fixture.detectChanges();

    expect(account.submitGame).toHaveBeenCalledWith('https://store.steampowered.com/app/2');
    expect(tile('New Game').querySelector('button')).toBeNull();
    expect(textOf(tile('New Game'))).toContain('Awaiting approval');
  });

  it('shows the server error if the request fails', () => {
    const submitGame = vi.fn().mockReturnValue(throwError(() => new HttpErrorResponse({ status: 403, error: { error: "you're not able to submit games" } })));
    const { fixture, el, tile } = render({ profile: profile(), submitGame });
    (tile('New Game').querySelector('button') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(textOf(el)).toContain("You're not able to submit games");
    expect(tile('New Game').querySelector('button')).not.toBeNull();
  });

  describe('wishlist import', () => {
    const imported = { wishlist_size: 4, watched: 2, already_watched: 0, requestable: [], awaiting_review: 0, unavailable: 0 };

    it('puts the Import wishlist button in its own strip under the profile card, leaving the card as it was', () => {
      const { el } = render({ profile: profile() });
      const strip = el.querySelector('.wishlist-strip')!;
      expect(textOf(strip.querySelector('.import-button'))).toBe('Import wishlist');
      const card = strip.previousElementSibling!;
      expect(card.classList.contains('edge-panel')).toBe(true);
      expect(card.contains(strip)).toBe(false);
      expect(card.querySelector('.import-button')).toBeNull();
      expect(card.querySelectorAll(':scope > div > div')).toHaveLength(2); // avatar and recently played only
    });

    it('does not show it to someone who has not linked Steam', () => {
      const { el } = render({ profile: null });
      expect(el.querySelector('.wishlist-strip')).toBeNull();
      expect(el.querySelector('app-wishlist-import')).toBeNull();
    });

    it('reloads the feed after games were added to the watchlist', () => {
      const { fixture, el, account } = render({ profile: profile() });
      account.importWishlist.mockReturnValue(of(imported));
      expect(account.getFeed).toHaveBeenCalledTimes(1);
      (el.querySelector('.import-button') as HTMLButtonElement).click();
      fixture.detectChanges();
      expect(account.importWishlist).toHaveBeenCalled();
      expect(account.getFeed).toHaveBeenCalledTimes(2);
    });

    it('does not reload the feed when nothing was added', () => {
      const { fixture, el, account } = render({ profile: profile() });
      account.importWishlist.mockReturnValue(of({ ...imported, watched: 0, already_watched: 4 }));
      (el.querySelector('.import-button') as HTMLButtonElement).click();
      fixture.detectChanges();
      expect(account.getFeed).toHaveBeenCalledTimes(1);
    });
  });

  it('explains when nothing was played recently', () => {
    const { el } = render({ profile: profile({ recently_played: [] }) });
    expect(textOf(el)).toContain('Nothing played recently');
  });

  it('invites unlinked users to link Steam', () => {
    const { el } = render({ profile: null });
    expect(textOf(el)).toContain('Link your Steam account');
    expect(el.querySelector('a[href="/account"]')).not.toBeNull();
  });

  it.each([
    [503, "Steam profiles aren't set up on this server yet."],
    [502, "Couldn't reach Steam right now."],
  ])('explains a %s from the profile endpoint', (status, message) => {
    expect(textOf(render({ profileError: status }).el)).toContain(message);
  });
});

describe('FeedComponent: notifications', () => {
  it('shows recent notifications with unread ones emphasised', () => {
    const { el } = render({ notifications: [notification(), notification({ notification_id: 2, message: 'Old news', read_at: '2026-09-30T00:00:00Z' })] });
    expect(textOf(el)).toContain('Hades dropped to $10');
    expect(textOf(el)).toContain('Old news');
  });

  it('shows at most eight notifications', () => {
    const many = Array.from({ length: 12 }, (_, i) => notification({ notification_id: i + 1, message: `note ${i + 1}` }));
    const { el } = render({ notifications: many });
    expect(textOf(el)).toContain('note 8');
    expect(textOf(el)).not.toContain('note 9');
  });

  it('refreshes notifications when the feed opens', () => {
    expect(render().account.loadNotifications).toHaveBeenCalled();
  });
});
