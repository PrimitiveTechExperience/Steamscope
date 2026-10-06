import { TestBed } from '@angular/core/testing';
import { HttpErrorResponse } from '@angular/common/http';
import { Observable, NEVER, of, throwError } from 'rxjs';

import { WISHLIST_POLL_MS, WishlistImportComponent } from './wishlist-import';
import { AccountService } from '../../services/account';
import { AuthService } from '../../services/auth';
import { WishlistImportResult, WishlistStatus } from '../../models/user';
import { fakeAuth, makeUser, textOf } from '../../../testing/factories';

function result(over: Partial<WishlistImportResult> = {}): WishlistImportResult {
  return { wishlist_size: 5, watched: 3, already_watched: 0, requestable: [], awaiting_review: 0, unavailable: 0, state: 'incomplete', ...over };
}

function status(over: Partial<WishlistStatus> = {}): WishlistStatus {
  return { state: 'incomplete', wishlist_size: 5, remaining: 3, waiting: 0, unavailable: 0, ...over };
}

const MISSING = [
  { app_id: 11, name: 'Missing Eleven' },
  { app_id: 12, name: '' },
  { app_id: 13, name: 'Missing Thirteen' },
];

function render(opts: { importResult?: Observable<WishlistImportResult>; request?: Observable<unknown>; admin?: boolean; status?: Observable<WishlistStatus> } = {}) {
  const account = {
    getWishlistStatus: vi.fn().mockReturnValue(opts.status ?? of(status())),
    importWishlist: vi.fn().mockReturnValue(opts.importResult ?? of(result())),
    requestWishlistGames: vi.fn().mockReturnValue(opts.request ?? of({ requested: 3, skipped: 0, status: 'awaiting_approval', state: 'waiting' })),
  };
  TestBed.configureTestingModule({
    imports: [WishlistImportComponent],
    providers: [
      { provide: AccountService, useValue: account },
      { provide: AuthService, useValue: fakeAuth(makeUser({ is_admin: !!opts.admin })) },
    ],
  });
  const fixture = TestBed.createComponent(WishlistImportComponent);
  const imported = vi.fn();
  fixture.componentInstance.imported.subscribe(imported);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const q = (s: string) => el.querySelector(s);
  const click = (s: string) => {
    (q(s) as HTMLElement).click();
    fixture.detectChanges();
  };
  return { fixture, el, account, imported, q, click };
}

describe('WishlistImportComponent', () => {
  it('offers an Import wishlist button and does nothing until it is pressed', () => {
    const { q, account } = render();
    expect(textOf(q('.import-button'))).toBe('Import wishlist');
    expect(account.importWishlist).not.toHaveBeenCalled();
    expect(q('.summary')).toBeNull();
    expect(q('.dialog')).toBeNull();
  });

  describe('importing', () => {
    it('imports, says how many games are now tracked, and tells the page to refresh', () => {
      const { q, click, account, imported } = render({ importResult: of(result({ watched: 3, already_watched: 2, state: 'complete' })) });
      click('.import-button');
      expect(account.importWishlist).toHaveBeenCalledTimes(1);
      expect(textOf(q('.summary'))).toBe('Now tracking 3 games from your wishlist. 2 were already tracked.');
      expect(imported).toHaveBeenCalledTimes(1);
      expect(q('.dialog')).toBeNull(); // nothing to ask about
    });

    it('uses the singular', () => {
      const { q, click } = render({ importResult: of(result({ watched: 1, already_watched: 1 })) });
      click('.import-button');
      expect(textOf(q('.summary'))).toBe('Now tracking 1 game from your wishlist. 1 was already tracked.');
    });

    it('does not refresh the page when nothing new was added', () => {
      const { q, click, imported } = render({ importResult: of(result({ watched: 0, already_watched: 4 })) });
      click('.import-button');
      expect(imported).not.toHaveBeenCalled();
      expect(textOf(q('.summary'))).toBe('4 were already tracked.');
    });

    it('mentions games already requested and games that cannot be added', () => {
      const { q, click } = render({ importResult: of(result({ watched: 1, awaiting_review: 2, unavailable: 1 })) });
      click('.import-button');
      expect(textOf(q('.summary'))).toBe("Now tracking 1 game from your wishlist. 2 already requested. 1 can't be added.");
    });

    it('explains an empty or private wishlist', () => {
      const { q, click, imported } = render({ importResult: of(result({ wishlist_size: 0, watched: 0 })) });
      click('.import-button');
      expect(textOf(q('.summary'))).toContain('empty or private');
      expect(textOf(q('.summary'))).toContain('public');
      expect(imported).not.toHaveBeenCalled();
    });

    it('disables the button while importing, so it cannot be pressed twice', () => {
      const { q, click, account } = render({ importResult: NEVER });
      click('.import-button');
      expect((q('.import-button') as HTMLButtonElement).disabled).toBe(true);
      expect(textOf(q('.import-button'))).toBe('Importing...');
      (q('.import-button') as HTMLButtonElement).click();
      expect(account.importWishlist).toHaveBeenCalledTimes(1);
    });

    it.each([
      [400, 'link your Steam account first', 'Link your Steam account first'],
      [429, 'you have done that a few times already, try again in a while', 'You have done that a few times already, try again in a while'],
      [502, 'could not reach Steam right now', 'Could not reach Steam right now'],
    ])('shows the server message for a %s', (status, message, shown) => {
      const { q, click } = render({ importResult: throwError(() => new HttpErrorResponse({ status, error: { error: message } })) });
      click('.import-button');
      expect(textOf(q('.error'))).toBe(shown);
      expect((q('.import-button') as HTMLButtonElement).disabled).toBe(false); // can try again
    });

    it('falls back to a generic message when the server gives none', () => {
      const { q, click } = render({ importResult: throwError(() => new HttpErrorResponse({ status: 0 })) });
      click('.import-button');
      expect(textOf(q('.error'))).toBe("Can't reach the server right now.");
    });
  });

  describe('games we do not have', () => {
    const withMissing = () => render({ importResult: of(result({ watched: 2, requestable: MISSING })) });

    it('opens a dialog asking whether to request them, after the known games were added', () => {
      const { q, click, imported } = withMissing();
      click('.import-button');
      expect(imported).toHaveBeenCalled(); // the watched games are in already
      const dialog = q('.dialog')!;
      expect(dialog.getAttribute('role')).toBe('dialog');
      expect(dialog.getAttribute('aria-modal')).toBe('true');
      expect(textOf(q('h2'))).toBe("3 games aren't tracked yet");
      expect(textOf(dialog)).toContain('An admin reviews each request first');
    });

    it('lists each game, naming those it could and falling back to the app id', () => {
      const { q, click } = withMissing();
      click('.import-button');
      const rows = Array.from(q('.games')!.querySelectorAll('li'));
      expect(rows.map((r) => textOf(r.querySelector('a')))).toEqual(['Missing Eleven', 'App 12', 'Missing Thirteen']);
      const link = rows[1].querySelector('a')!;
      expect(link.getAttribute('href')).toBe('https://store.steampowered.com/app/12');
      expect(link.getAttribute('rel')).toContain('noopener');
    });

    it('requests every game by default', () => {
      const { q, click, account } = withMissing();
      click('.import-button');
      expect(textOf(q('.confirm'))).toBe('Request 3 games');
      click('.confirm');
      expect(account.requestWishlistGames).toHaveBeenCalledWith([11, 12, 13], []);
    });

    it('lets the user untick games, and select all or none', () => {
      const { q, click, account, fixture } = withMissing();
      click('.import-button');
      const boxes = () => Array.from(q('.games')!.querySelectorAll<HTMLInputElement>('input.request-box'));
      boxes()[1].click();
      fixture.detectChanges();
      expect(textOf(q('.confirm'))).toBe('Request 2 games');
      click('.confirm');
      expect(account.requestWishlistGames).toHaveBeenCalledWith([11, 13], []);
    });

    it('disables the request button when nothing is selected', () => {
      const { q, click, fixture, account } = withMissing();
      click('.import-button');
      click('.select-none');
      expect((q('.confirm') as HTMLButtonElement).disabled).toBe(true);
      expect(textOf(q('.confirm'))).toBe('Request 0 games');
      (q('.confirm') as HTMLButtonElement).click();
      expect(account.requestWishlistGames).not.toHaveBeenCalled();
      click('.select-all');
      fixture.detectChanges();
      expect(textOf(q('.confirm'))).toBe('Request 3 games');
    });

    it('closes without requesting on "No thanks", leaving a way to come back', () => {
      const { q, click, account } = withMissing();
      click('.import-button');
      click('.decline');
      expect(q('.dialog')).toBeNull();
      expect(account.requestWishlistGames).not.toHaveBeenCalled();
      expect(textOf(q('.reopen'))).toContain("3 games aren't on Steamscope yet");
      click('.reopen');
      expect(q('.dialog')).not.toBeNull();
    });

    it('closes on Escape and on a click outside the dialog, but not on a click inside it', () => {
      const { q, click, fixture } = withMissing();
      click('.import-button');
      (q('.dialog') as HTMLElement).click();
      fixture.detectChanges();
      expect(q('.dialog')).not.toBeNull();
      q('.dialog-backdrop')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
      fixture.detectChanges();
      expect(q('.dialog')).toBeNull();
      click('.reopen');
      (q('.dialog-backdrop') as HTMLElement).click();
      fixture.detectChanges();
      expect(q('.dialog')).toBeNull();
    });

    it('confirms the request and closes the dialog', () => {
      const { q, click } = withMissing();
      click('.import-button');
      click('.confirm');
      expect(q('.dialog')).toBeNull();
      expect(textOf(q('.request-note'))).toBe("Requested 3 games. They'll be added to your watchlist once approved.");
      expect(q('.reopen')).toBeNull();
    });

    it('says admins requests are added right away', () => {
      const { q, click } = render({ importResult: of(result({ requestable: MISSING })), request: of({ requested: 3, skipped: 0, status: 'pending', state: 'waiting' }), admin: true });
      click('.import-button');
      expect(textOf(q('.dialog'))).toContain('scraped right away');
      click('.confirm');
      expect(textOf(q('.request-note'))).toBe('Requested 3 games; they are being added now.');
    });

    it('says so when nothing new was requested', () => {
      const { q, click } = render({ importResult: of(result({ requestable: MISSING })), request: of({ requested: 0, skipped: 3, status: 'awaiting_approval', state: 'waiting' }) });
      click('.import-button');
      click('.confirm');
      expect(textOf(q('.request-note'))).toContain('Nothing new was requested');
    });

    it('keeps the dialog open and shows the error when the request fails', () => {
      const { q, click } = render({
        importResult: of(result({ requestable: MISSING })),
        request: throwError(() => new HttpErrorResponse({ status: 429, error: { error: 'you have done that a few times already, try again in a while' } })),
      });
      click('.import-button');
      click('.confirm');
      expect(q('.dialog')).not.toBeNull();
      expect(textOf(q('.error'))).toBe('You have done that a few times already, try again in a while');
      expect((q('.confirm') as HTMLButtonElement).disabled).toBe(false);
    });

    it('cannot be dismissed while the request is in flight', () => {
      const { q, click } = render({ importResult: of(result({ requestable: MISSING })), request: NEVER });
      click('.import-button');
      click('.confirm');
      expect(textOf(q('.confirm'))).toBe('Requesting...');
      expect((q('.decline') as HTMLButtonElement).disabled).toBe(true);
      (q('.dialog-backdrop') as HTMLElement).click();
      expect(q('.dialog')).not.toBeNull();
    });

    it('uses the singular for one game', () => {
      const { q, click } = render({ importResult: of(result({ requestable: [MISSING[0]] })) });
      click('.import-button');
      expect(textOf(q('h2'))).toBe("1 game isn't tracked yet");
      expect(textOf(q('.confirm'))).toBe('Request 1 game');
    });
  });

  describe('the pin option', () => {
    const open = () => {
      const r = render({ importResult: of(result({ requestable: MISSING })) });
      r.click('.import-button');
      const pins = () => Array.from(r.q('.games')!.querySelectorAll<HTMLInputElement>('input.pin-box'));
      const boxes = () => Array.from(r.q('.games')!.querySelectorAll<HTMLInputElement>('input.request-box'));
      return { ...r, pins, boxes };
    };

    it('offers a Pin box for each game, unchecked by default', () => {
      const { pins, q } = open();
      expect(pins()).toHaveLength(3);
      expect(pins().every((p) => !p.checked)).toBe(true);
      expect(textOf(q('.dialog'))).toContain('tick Pin to put one at the top of your feed');
      expect(textOf(q('.pin-count'))).toBe('');
    });

    it('sends the pinned games along with the request', () => {
      const { pins, fixture, q, click, account } = open();
      pins()[0].click();
      pins()[2].click();
      fixture.detectChanges();
      expect(textOf(q('.pin-count'))).toBe('2 will be pinned');
      click('.confirm');
      expect(account.requestWishlistGames).toHaveBeenCalledWith([11, 12, 13], [11, 13]);
    });

    it('pins and unpins everything at once', () => {
      const { pins, fixture, q, click, account } = open();
      expect(textOf(q('.pin-all'))).toBe('Pin all');
      click('.pin-all');
      expect(pins().every((p) => p.checked)).toBe(true);
      expect(textOf(q('.pin-all'))).toBe('Unpin all');
      click('.pin-all');
      fixture.detectChanges();
      expect(pins().every((p) => !p.checked)).toBe(true);
      click('.pin-all');
      click('.confirm');
      expect(account.requestWishlistGames).toHaveBeenCalledWith([11, 12, 13], [11, 12, 13]);
    });

    it('does not pin a game that is not being requested', () => {
      const { pins, boxes, fixture, click, account } = open();
      pins()[1].click();
      boxes()[1].click(); // untick the game that was pinned
      fixture.detectChanges();
      expect(pins()[1].disabled).toBe(true);
      click('.confirm');
      expect(account.requestWishlistGames).toHaveBeenCalledWith([11, 13], []);
    });

    it('Pin all only applies to the games that are ticked', () => {
      const { boxes, click, account } = open();
      boxes()[0].click();
      click('.pin-all');
      click('.confirm');
      expect(account.requestWishlistGames).toHaveBeenCalledWith([12, 13], [12, 13]);
    });

    it('disables Pin all when nothing is selected', () => {
      const { q, click } = open();
      click('.select-none');
      expect((q('.pin-all') as HTMLButtonElement).disabled).toBe(true);
    });

    it('forgets pin choices when importing again', () => {
      const { pins, fixture, click, account } = open();
      pins()[0].click();
      click('.decline');
      fixture.detectChanges();
      click('.import-button');
      expect(pins().every((p) => !p.checked)).toBe(true);
      click('.confirm');
      expect(account.requestWishlistGames).toHaveBeenLastCalledWith([11, 12, 13], []);
    });
  });

  describe('when everything has been imported', () => {
    it('shows a note instead of the button', () => {
      const { q } = render({ status: of(status({ state: 'complete', remaining: 0 })) });
      expect(q('.import-button')).toBeNull();
      expect(textOf(q('.all-imported'))).toContain('All your wishlist games are imported.');
      expect(q('.unavailable-note')).toBeNull();
    });

    it('mentions games that cannot be added', () => {
      const { q } = render({ status: of(status({ state: 'complete', remaining: 0, unavailable: 2 })) });
      expect(textOf(q('.unavailable-note'))).toBe("(2 can't be added.)");
    });

    it('replaces the button once an import finishes the job', () => {
      const { q, click } = render({ importResult: of(result({ watched: 5, state: 'complete' })) });
      expect(q('.import-button')).not.toBeNull();
      click('.import-button');
      expect(q('.import-button')).toBeNull();
      expect(textOf(q('.all-imported'))).toContain('All your wishlist games are imported.');
      expect(textOf(q('.summary'))).toBe('Now tracking 5 games from your wishlist.');
    });

    it('does the same once the requested games are in', () => {
      const { q, click } = render({ importResult: of(result({ requestable: MISSING })) });
      click('.import-button');
      click('.confirm'); // the default request reply says the state is now "waiting"
      expect(q('.import-button')).toBeNull();
      expect(textOf(q('.waiting-note'))).toContain('3 wishlist games are waiting to be added.');
    });

    it('keeps the button while games still need importing', () => {
      const { q } = render({ status: of(status({ state: 'incomplete' })) });
      expect(q('.import-button')).not.toBeNull();
      expect(q('.all-imported')).toBeNull();
    });
  });

  describe('when requested games are still being added', () => {
    it('says so, with no button, and uses the singular for one', () => {
      const { q } = render({ status: of(status({ state: 'waiting', remaining: 0, waiting: 1 })) });
      expect(q('.import-button')).toBeNull();
      expect(textOf(q('.waiting-note'))).toBe("1 wishlist game is waiting to be added. They'll be tracked automatically.");
    });
  });

  describe('checking the wishlist when the page opens', () => {
    it('asks the server once and shows no button until it answers', () => {
      const { q, account } = render({ status: NEVER });
      expect(account.getWishlistStatus).toHaveBeenCalledTimes(1);
      expect(textOf(q('.checking'))).toBe('Checking your wishlist...');
      expect(q('.import-button')).toBeNull();
      expect(q('.all-imported')).toBeNull();
    });

    it('does not change anything by checking', () => {
      const { account } = render();
      expect(account.importWishlist).not.toHaveBeenCalled();
      expect(account.requestWishlistGames).not.toHaveBeenCalled();
    });

    it.each([['empty'], ['incomplete']] as const)('offers the button for a %s wishlist', (state) => {
      const { q } = render({ status: of(status({ state })) });
      expect(textOf(q('.import-button'))).toBe('Import wishlist');
    });

    it('offers the button when the check fails, since importing may still work', () => {
      const { q } = render({ status: throwError(() => new HttpErrorResponse({ status: 502 })) });
      expect(q('.checking')).toBeNull();
      expect(textOf(q('.import-button'))).toBe('Import wishlist');
    });
  });

  describe('updating without a reload', () => {
    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    const waiting = (n: number) => status({ state: 'waiting', remaining: 0, waiting: n });
    const done = () => status({ state: 'complete', remaining: 0, waiting: 0 });

    async function tick(fixture: { detectChanges(): void }, ms = WISHLIST_POLL_MS) {
      await vi.advanceTimersByTimeAsync(ms);
      fixture.detectChanges();
    }

    it('switches from "waiting" to "all imported" by itself once the games are added', async () => {
      const { fixture, q, account, imported } = render({ status: of(waiting(2)) });
      expect(textOf(q('.waiting-note'))).toContain('2 wishlist games are waiting');
      account.getWishlistStatus.mockReturnValue(of(done()));

      await tick(fixture);

      expect(q('.waiting-note')).toBeNull();
      expect(textOf(q('.all-imported'))).toContain('All your wishlist games are imported.');
      expect(q('.import-button')).toBeNull();
      expect(imported).toHaveBeenCalledTimes(1); // so the page can show the new games in the watchlist
    });

    it('goes live straight after requesting games, with no reload', async () => {
      const { fixture, q, click, account, imported } = render({ importResult: of(result({ requestable: MISSING })) });
      click('.import-button');
      click('.confirm');
      expect(textOf(q('.waiting-note'))).toContain('3 wishlist games are waiting');

      account.getWishlistStatus.mockReturnValue(of(done()));
      await tick(fixture);

      expect(textOf(q('.all-imported'))).toContain('All your wishlist games are imported.');
      expect(imported).toHaveBeenCalledTimes(2); // once by the import itself, once when the requested games arrived
    });

    it('updates the count as games arrive, and tells the page each time', async () => {
      const { fixture, q, account, imported } = render({ status: of(waiting(3)) });
      account.getWishlistStatus.mockReturnValue(of(waiting(2)));
      await tick(fixture);
      expect(textOf(q('.waiting-note'))).toContain('2 wishlist games are waiting');
      expect(imported).toHaveBeenCalledTimes(1);

      await tick(fixture); // nothing changed
      expect(imported).toHaveBeenCalledTimes(1);

      account.getWishlistStatus.mockReturnValue(of(done()));
      await tick(fixture);
      expect(textOf(q('.all-imported'))).toBeTruthy();
      expect(imported).toHaveBeenCalledTimes(2);
    });

    it('stops asking once it is no longer waiting', async () => {
      const { fixture, account } = render({ status: of(waiting(1)) });
      account.getWishlistStatus.mockReturnValue(of(done()));
      await tick(fixture);
      const calls = account.getWishlistStatus.mock.calls.length;
      await tick(fixture, WISHLIST_POLL_MS * 5);
      expect(account.getWishlistStatus).toHaveBeenCalledTimes(calls);
    });

    it('does not ask at all when there is nothing to wait for', async () => {
      for (const state of ['incomplete', 'complete', 'empty'] as const) {
        TestBed.resetTestingModule();
        const { fixture, account } = render({ status: of(status({ state })) });
        await tick(fixture, WISHLIST_POLL_MS * 3);
        expect(account.getWishlistStatus, state).toHaveBeenCalledTimes(1); // just the first check
      }
    });

    it('brings the button back if the game is deleted while waiting', async () => {
      const { fixture, q, account } = render({ status: of(waiting(1)) });
      account.getWishlistStatus.mockReturnValue(of(status({ state: 'incomplete', remaining: 1, waiting: 0 })));
      await tick(fixture);
      expect(q('.waiting-note')).toBeNull();
      expect(textOf(q('.import-button'))).toBe('Import wishlist');
    });

    it('keeps trying after a failed check', async () => {
      const { fixture, q, account } = render({ status: of(waiting(1)) });
      account.getWishlistStatus.mockReturnValue(throwError(() => new HttpErrorResponse({ status: 502 })));
      await tick(fixture);
      expect(textOf(q('.waiting-note'))).toContain('1 wishlist game is waiting'); // unchanged
      account.getWishlistStatus.mockReturnValue(of(done()));
      await tick(fixture);
      expect(textOf(q('.all-imported'))).toBeTruthy();
    });

    it('stops when the component goes away', async () => {
      const { fixture, account } = render({ status: of(waiting(1)) });
      fixture.destroy();
      const calls = account.getWishlistStatus.mock.calls.length;
      await vi.advanceTimersByTimeAsync(WISHLIST_POLL_MS * 4);
      expect(account.getWishlistStatus).toHaveBeenCalledTimes(calls);
    });
  });
});
