import { Injectable, computed, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, catchError, of, tap } from 'rxjs';

import { API_URL } from '../api';
import {
  AppNotification,
  Feed,
  Preferences,
  RecentSearch,
  SteamProfile,
  Submission,
  SubmissionStatus,
  WatchedGame,
  WishlistImportResult,
  WishlistRequestResult,
  WishlistStatus,
} from '../models/user';

@Injectable({ providedIn: 'root' })
export class AccountService {
  private http = inject(HttpClient);
  private base = `${API_URL}/me`;

  readonly notifications = signal<AppNotification[]>([]);
  readonly unreadCount = computed(() => this.notifications().filter((n) => !n.read_at).length);

  getPreferences(): Observable<Preferences> {
    return this.http.get<Preferences>(`${this.base}/preferences`);
  }

  updatePreferences(prefs: Preferences): Observable<Preferences> {
    return this.http.put<Preferences>(`${this.base}/preferences`, prefs);
  }

  getWatchlist(): Observable<WatchedGame[]> {
    return this.http.get<WatchedGame[]>(`${this.base}/watchlist`);
  }

  watch(appId: number, pinned: boolean, targetPrice: number | null): Observable<void> {
    return this.http.put<void>(`${this.base}/watchlist/${appId}`, { pinned, target_price: targetPrice });
  }

  unwatch(appId: number): Observable<void> {
    return this.http.delete<void>(`${this.base}/watchlist/${appId}`);
  }

  loadNotifications(): void {
    this.http
      .get<AppNotification[]>(`${this.base}/notifications`)
      .pipe(catchError(() => of([] as AppNotification[])))
      .subscribe((list) => this.notifications.set(list));
  }

  markAllRead(): Observable<void> {
    return this.http.post<void>(`${this.base}/notifications/read`, {}).pipe(
      tap(() => {
        const now = new Date().toISOString();
        this.notifications.update((list) => list.map((n) => (n.read_at ? n : { ...n, read_at: now })));
      })
    );
  }

  markRead(id: number): Observable<void> {
    return this.http.post<void>(`${this.base}/notifications/read`, { ids: [id] }).pipe(
      tap(() => {
        const now = new Date().toISOString();
        this.notifications.update((list) =>
          list.map((n) => (n.notification_id === id && !n.read_at ? { ...n, read_at: now } : n))
        );
      })
    );
  }

  clearNotifications(): void {
    this.notifications.set([]);
  }

  getFeed(): Observable<Feed> {
    return this.http.get<Feed>(`${this.base}/feed`);
  }

  getSteamProfile(): Observable<{ profile: SteamProfile | null }> {
    return this.http.get<{ profile: SteamProfile | null }>(`${this.base}/steam-profile`);
  }

  /** Watches every wishlisted game we have, and lists the ones we do not. */
  importWishlist(): Observable<WishlistImportResult> {
    return this.http.post<WishlistImportResult>(`${this.base}/wishlist/import`, {});
  }

  /** Where the wishlist stands, without changing anything. */
  getWishlistStatus(): Observable<WishlistStatus> {
    return this.http.get<WishlistStatus>(`${this.base}/wishlist/status`);
  }

  /**
   * Asks for wishlisted games we do not track to be added. Once added they are
   * watched, and pinned to the feed if listed in `pinnedIds`.
   */
  requestWishlistGames(appIds: number[], pinnedIds: number[] = []): Observable<WishlistRequestResult> {
    const body = pinnedIds.length > 0 ? { app_ids: appIds, pinned_app_ids: pinnedIds } : { app_ids: appIds };
    return this.http.post<WishlistRequestResult>(`${this.base}/wishlist/request`, body);
  }

  unlinkSteam(): Observable<void> {
    return this.http.delete<void>(`${this.base}/steam`);
  }

  getRecentSearches(): Observable<RecentSearch[]> {
    return this.http.get<RecentSearch[]>(`${this.base}/recent-searches`);
  }

  addRecentSearch(search: RecentSearch): Observable<void> {
    return this.http.post<void>(`${this.base}/recent-searches`, search);
  }

  getSubmissions(): Observable<Submission[]> {
    return this.http.get<Submission[]>(`${this.base}/submissions`);
  }

  submitGame(url: string): Observable<{ kind: Submission['kind']; id: number; status: SubmissionStatus }> {
    return this.http.post<{ kind: Submission['kind']; id: number; status: SubmissionStatus }>(`${API_URL}/submissions`, { url });
  }
}
