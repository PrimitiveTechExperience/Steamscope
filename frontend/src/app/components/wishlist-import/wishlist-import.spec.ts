import { TestBed } from '@angular/core/testing';
import { HttpErrorResponse } from '@angular/common/http';
import { Observable, NEVER, of, throwError } from 'rxjs';

import { WishlistImportComponent } from './wishlist-import';
import { AccountService } from '../../services/account';
import { AuthService } from '../../services/auth';
import { WishlistImportResult } from '../../models/user';
import { fakeAuth, makeUser, textOf } from '../../../testing/factories';

function result(over: Partial<WishlistImportResult> = {}): WishlistImportResult {
  return { wishlist_size: 5, watched: 3, already_watched: 0, requestable: [], awaiting_review: 0, unavailable: 0, ...over };
}

const MISSING = [
  { app_id: 11, name: 'Missing Eleven' },
  { app_id: 12, name: '' },
  { app_id: 13, name: 'Missing Thirteen' },
];

function render(opts: { importResult?: Observable<WishlistImportResult>; request?: Observable<unknown>; admin?: boolean } = {}) {
  const account = {
    importWishlist: vi.fn().mockReturnValue(opts.importResult ?? of(result())),
    requestWishlistGames: vi.fn().mockReturnValue(opts.request ?? of({ requested: 3, skipped: 0, status: 'awaiting_approval' })),
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
      const { q, click, account, imported } = render({ importResult: of(result({ watched: 3, already_watched: 2 })) });
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
      expect(rows.map((r) => textOf(r))).toEqual(['Missing Eleven', 'App 12', 'Missing Thirteen']);
      const link = rows[1].querySelector('a')!;
      expect(link.getAttribute('href')).toBe('https://store.steampowered.com/app/12');
      expect(link.getAttribute('rel')).toContain('noopener');
    });

    it('requests every game by default', () => {
      const { q, click, account } = withMissing();
      click('.import-button');
      expect(textOf(q('.confirm'))).toBe('Request 3 games');
      click('.confirm');
      expect(account.requestWishlistGames).toHaveBeenCalledWith([11, 12, 13]);
    });

    it('lets the user untick games, and select all or none', () => {
      const { q, click, account, fixture } = withMissing();
      click('.import-button');
      const boxes = () => Array.from(q('.games')!.querySelectorAll<HTMLInputElement>('input[type="checkbox"]'));
      boxes()[1].click();
      fixture.detectChanges();
      expect(textOf(q('.confirm'))).toBe('Request 2 games');
      click('.confirm');
      expect(account.requestWishlistGames).toHaveBeenCalledWith([11, 13]);
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
      expect(textOf(q('.request-note'))).toBe("Requested 3 games. You'll get a notification when they're added.");
      expect(q('.reopen')).toBeNull();
    });

    it('says admins requests are added right away', () => {
      const { q, click } = render({ importResult: of(result({ requestable: MISSING })), request: of({ requested: 3, skipped: 0, status: 'pending' }), admin: true });
      click('.import-button');
      expect(textOf(q('.dialog'))).toContain('scraped right away');
      click('.confirm');
      expect(textOf(q('.request-note'))).toBe('Requested 3 games; they are being added now.');
    });

    it('says so when nothing new was requested', () => {
      const { q, click } = render({ importResult: of(result({ requestable: MISSING })), request: of({ requested: 0, skipped: 3, status: 'awaiting_approval' }) });
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
});
