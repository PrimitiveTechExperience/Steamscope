import { Component, computed, inject, output, signal } from '@angular/core';

import { AccountService } from '../../services/account';
import { AuthService } from '../../services/auth';
import { WishlistGame, WishlistImportResult } from '../../models/user';
import { apiErrorMessage } from '../../api';

/**
 * "Import wishlist": watches every wishlisted game we already have, then
 * offers (in a dialog) to request the ones we do not.
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

  protected importing = signal(false);
  protected requesting = signal(false);
  protected result = signal<WishlistImportResult | null>(null);
  protected error = signal<string | null>(null);
  /** What happened when games were requested, shown after the dialog closes. */
  protected requestNote = signal<string | null>(null);

  protected dialogOpen = signal(false);
  /** App IDs unticked in the dialog; everything else is requested. */
  private unticked = signal<ReadonlySet<number>>(new Set());

  protected requestable = computed<WishlistGame[]>(() => this.result()?.requestable ?? []);
  protected selectedIds = computed(() => this.requestable().filter((g) => !this.unticked().has(g.app_id)).map((g) => g.app_id));

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

  protected toggle(game: WishlistGame) {
    const next = new Set(this.unticked());
    if (!next.delete(game.app_id)) next.add(game.app_id);
    this.unticked.set(next);
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
        this.unticked.set(new Set());
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
    this.account.requestWishlistGames(ids).subscribe({
      next: (res) => {
        this.requesting.set(false);
        this.dialogOpen.set(false);
        this.requestNote.set(
          res.requested === 0
            ? 'Nothing new was requested; those games were already on their way.'
            : res.status === 'pending'
              ? `Requested ${res.requested} ${res.requested === 1 ? 'game' : 'games'}; they are being added now.`
              : `Requested ${res.requested} ${res.requested === 1 ? 'game' : 'games'}. You'll get a notification when they're added.`
        );
      },
      error: (err) => {
        this.requesting.set(false);
        this.error.set(apiErrorMessage(err, "Couldn't request those games."));
      },
    });
  }
}
