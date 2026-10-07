import { Component, PLATFORM_ID, computed, inject, output, signal } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { takeUntilDestroyed, toObservable } from '@angular/core/rxjs-interop';
import { EMPTY, catchError, switchMap, timer } from 'rxjs';

import { AccountService } from '../../services/account';
import { AuthService } from '../../services/auth';
import { WishlistGame, WishlistImportResult, WishlistState, WishlistStatus } from '../../models/user';
import { apiErrorMessage } from '../../api';

/** While requested games are being added, ask again this often (ms). */
export const WISHLIST_POLL_MS = 20_000;

/**
 * "Import wishlist": watches every wishlisted game we already have, then
 * offers (in a dialog) to request the ones we do not, optionally pinning each
 * to the feed. Once everything is imported the button is replaced by a note
 * saying so.
 */
@Component({
  selector: 'app-wishlist-import',
  templateUrl: './wishlist-import.html',
})
export class WishlistImportComponent {
  private account = inject(AccountService);
  protected auth = inject(AuthService);

  /** Emitted once games have been added to the watchlist, so the page can refresh. */
  imported = output<void>();

  /** Until the server answers we do not know whether to offer the button. */
  protected checking = signal(false);
  /** Where the wishlist stands; null when unknown, which shows the button. */
  protected state = signal<WishlistState | null>(null);
  /** Requested games that are still being added. */
  protected waiting = signal(0);
  /** Games that can never be added. */
  protected unavailable = signal(0);

  protected importing = signal(false);
  protected requesting = signal(false);
  protected result = signal<WishlistImportResult | null>(null);
  protected error = signal<string | null>(null);
  /** What happened when games were requested, shown after the dialog closes. */
  protected requestNote = signal<string | null>(null);

  protected dialogOpen = signal(false);
  /** App IDs unticked in the dialog; everything else is requested. */
  private unticked = signal<ReadonlySet<number>>(new Set());
  /** App IDs to pin to the feed once they are added. */
  private pinned = signal<ReadonlySet<number>>(new Set());

  protected requestable = computed<WishlistGame[]>(() => this.result()?.requestable ?? []);
  protected selectedIds = computed(() => this.requestable().filter((g) => !this.unticked().has(g.app_id)).map((g) => g.app_id));
  /** Pinned games among those being requested. */
  protected pinnedIds = computed(() => this.selectedIds().filter((id) => this.pinned().has(id)));
  protected allPinned = computed(() => this.selectedIds().length > 0 && this.pinnedIds().length === this.selectedIds().length);

  /** The button is only offered while there is something left to import. */
  protected showButton = computed(() => !this.checking() && this.state() !== 'complete' && this.state() !== 'waiting');

  constructor() {
    if (!isPlatformBrowser(inject(PLATFORM_ID))) return;
    this.checking.set(true);
    this.account.getWishlistStatus().subscribe({
      next: (status) => {
        this.checking.set(false);
        this.applyStatus(status);
      },
      // If we cannot tell, offer the button: importing works either way.
      error: () => this.checking.set(false),
    });

    // While requested games are waiting to be added, keep checking, so the
    // strip changes to "all imported" by itself once an admin has approved them.
    toObservable(this.state)
      .pipe(
        switchMap((state) =>
          state === 'waiting'
            ? timer(WISHLIST_POLL_MS, WISHLIST_POLL_MS).pipe(
                switchMap(() => this.account.getWishlistStatus().pipe(catchError(() => EMPTY)))
              )
            : EMPTY
        ),
        takeUntilDestroyed()
      )
      .subscribe((status) => {
        const added = status.waiting < this.waiting() || status.state === 'complete';
        this.applyStatus(status);
        // Games were added to the watchlist in the meantime; let the page show them.
        if (added) this.imported.emit();
      });
  }

  private applyStatus(status: WishlistStatus) {
    this.state.set(status.state);
    this.waiting.set(status.waiting);
    this.unavailable.set(status.unavailable);
  }

  protected summary = computed(() => {
    const r = this.result();
    if (!r) return null;
    if (r.wishlist_size === 0) {
      return 'No wishlist found. It is either empty or private; set your Steam game details to public and try again.';
    }
    const parts: string[] = [];
    if (r.watched > 0) parts.push(`Now tracking ${r.watched} ${r.watched === 1 ? 'game' : 'games'} from your wishlist.`);
    if (r.already_watched > 0) parts.push(`${r.already_watched} ${r.already_watched === 1 ? 'was' : 'were'} already tracked.`);
    if (r.awaiting_review > 0) parts.push(`${r.awaiting_review} already requested.`);
    if (r.unavailable > 0) parts.push(`${r.unavailable} can't be added.`);
    if (parts.length === 0) parts.push('Nothing new to track.');
    return parts.join(' ');
  });

  protected label(game: WishlistGame): string {
    return game.name || `App ${game.app_id}`;
  }

  protected storeUrl(game: WishlistGame): string {
    return `https://store.steampowered.com/app/${game.app_id}`;
  }

  protected isTicked(game: WishlistGame): boolean {
    return !this.unticked().has(game.app_id);
  }

  protected isPinned(game: WishlistGame): boolean {
    return this.pinned().has(game.app_id);
  }

  protected toggle(game: WishlistGame) {
    const next = new Set(this.unticked());
    if (!next.delete(game.app_id)) next.add(game.app_id);
    this.unticked.set(next);
  }

  protected togglePin(game: WishlistGame) {
    const next = new Set(this.pinned());
    if (!next.delete(game.app_id)) next.add(game.app_id);
    this.pinned.set(next);
  }

  /** Pins every selected game, or unpins them all if they already are. */
  protected togglePinAll() {
    const pinAll = !this.allPinned();
    const next = new Set(this.pinned());
    for (const id of this.selectedIds()) {
      if (pinAll) next.add(id);
      else next.delete(id);
    }
    this.pinned.set(next);
  }

  protected setAll(ticked: boolean) {
    this.unticked.set(ticked ? new Set() : new Set(this.requestable().map((g) => g.app_id)));
  }

  protected importWishlist() {
    if (this.importing()) return;
    this.importing.set(true);
    this.error.set(null);
    this.requestNote.set(null);
    this.account.importWishlist().subscribe({
      next: (result) => {
        this.importing.set(false);
        this.result.set(result);
        this.state.set(result.state);
        this.waiting.set(result.awaiting_review);
        this.unavailable.set(result.unavailable);
        this.unticked.set(new Set());
        this.pinned.set(new Set());
        if (result.watched > 0) this.imported.emit();
        if (result.requestable.length > 0) this.dialogOpen.set(true);
      },
      error: (err) => {
        this.importing.set(false);
        this.result.set(null);
        this.error.set(apiErrorMessage(err, "Couldn't import your wishlist."));
      },
    });
  }

  protected closeDialog() {
    if (!this.requesting()) this.dialogOpen.set(false);
  }

  protected request() {
    const ids = this.selectedIds();
    if (ids.length === 0 || this.requesting()) return;
    this.requesting.set(true);
    this.error.set(null);
    this.account.requestWishlistGames(ids, this.pinnedIds()).subscribe({
      next: (res) => {
        this.requesting.set(false);
        this.dialogOpen.set(false);
        this.state.set(res.state);
        this.waiting.set(res.requested);
        this.requestNote.set(
          res.requested === 0
            ? 'Nothing new was requested; those games were already on their way.'
            : res.status === 'pending'
              ? `Requested ${res.requested} ${res.requested === 1 ? 'game' : 'games'}; they are being added now.`
              : `Requested ${res.requested} ${res.requested === 1 ? 'game' : 'games'}. They'll be added to your watchlist once approved.`
        );
      },
      error: (err) => {
        this.requesting.set(false);
        this.error.set(apiErrorMessage(err, "Couldn't request those games."));
      },
    });
  }
}
