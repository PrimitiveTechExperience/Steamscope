import { Component, computed, inject, signal } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { CurrencyPipe, DatePipe, DecimalPipe } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { toSignal } from '@angular/core/rxjs-interop';
import { catchError, map, of } from 'rxjs';

import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { GameCardComponent } from '../../components/game-card/game-card';
import { RevealOnScrollDirective } from '../../directives/reveal-on-scroll';
import { AppNotification, Feed, NotificationKind, SteamPlayedGame, SteamProfile, SubmissionStatus } from '../../models/user';
import { apiErrorMessage } from '../../api';
import { withLoading } from '../../utils/with-loading';

type ProfileState =
  | { kind: 'profile'; profile: SteamProfile }
  | { kind: 'not-linked' }
  | { kind: 'unavailable'; reason: string };

const EMPTY_FEED: Feed = { watchlist: [], deals: [], suggestions: [] };

function greetingFor(hour: number): string {
  if (hour < 5) return 'Up late';
  if (hour < 12) return 'Good morning';
  if (hour < 18) return 'Good afternoon';
  return 'Good evening';
}

@Component({
  selector: 'app-feed',
  imports: [RouterLink, CurrencyPipe, DatePipe, DecimalPipe, GameCardComponent, RevealOnScrollDirective],
  templateUrl: './feed.html',
})
export class FeedComponent {
  protected auth = inject(AuthService);
  protected account = inject(AccountService);
  private router = inject(Router);

  protected greeting = greetingFor(new Date().getHours());

  constructor() {
    // The header bell polls once a minute; load fresh ones on arrival.
    this.account.loadNotifications();
  }

  /** Track requests made from this page, so tiles update without a reload. */
  private trackOverrides = signal<Record<number, SubmissionStatus>>({});
  protected trackError = signal<string | null>(null);

  protected trackStatus(game: SteamPlayedGame): SubmissionStatus | '' {
    return this.trackOverrides()[game.app_id] ?? game.track_status;
  }

  protected trackLabel(status: SubmissionStatus | ''): string {
    switch (status) {
      case 'tracked':
        return 'Tracked';
      case 'awaiting_approval':
        return 'Awaiting approval';
      case 'pending':
        return 'Fetching...';
      case 'rejected':
        return 'Not approved';
      default:
        return '';
    }
  }

  /** Asks for a played game to be added to tracking (admins get it added directly). */
  protected requestTrack(game: SteamPlayedGame) {
    this.trackError.set(null);
    this.account.submitGame(`https://store.steampowered.com/app/${game.app_id}`).subscribe({
      next: (res) => this.trackOverrides.update((o) => ({ ...o, [game.app_id]: res.status })),
      error: (err) => this.trackError.set(apiErrorMessage(err, "Couldn't submit that game.")),
    });
  }

  protected recentNotifications = computed(() => this.account.notifications().slice(0, 8));

  protected notificationIcon(kind: NotificationKind): string {
    switch (kind) {
      case 'price_drop':
        return '\u{1F4C9}';
      case 'target_price':
        return '\u{1F3AF}';
      case 'submission_tracked':
      case 'bundle_tracked':
        return '\u2705';
      case 'submission_rejected':
        return '\u26D4';
      case 'submission_failed':
      case 'bundle_failed':
        return '\u26A0\uFE0F';
    }
  }

  protected openNotification(n: AppNotification) {
    if (!n.read_at) this.account.markRead(n.notification_id).subscribe();
    if (n.app_id) this.router.navigate(['/games', n.app_id]);
  }

  protected markAllRead() {
    this.account.markAllRead().subscribe();
  }

  private feedState = toSignal(
    withLoading(this.account.getFeed().pipe(catchError(() => of(EMPTY_FEED))), EMPTY_FEED),
    { initialValue: { data: EMPTY_FEED, loading: true } }
  );
  protected feed = computed(() => this.feedState().data);
  protected feedLoading = computed(() => this.feedState().loading);

  private profileState = toSignal(
    withLoading(
      this.account.getSteamProfile().pipe(
        map((res): ProfileState => (res.profile ? { kind: 'profile', profile: res.profile } : { kind: 'not-linked' })),
        catchError((err: HttpErrorResponse) =>
          of<ProfileState>({
            kind: 'unavailable',
            reason: err.status === 503 ? "Steam profiles aren't set up on this server yet." : "Couldn't reach Steam right now.",
          })
        )
      ),
      { kind: 'not-linked' } as ProfileState
    ),
    { initialValue: { data: { kind: 'not-linked' } as ProfileState, loading: true } }
  );
  protected profileLoading = computed(() => this.profileState().loading);
  protected steamProfile = computed(() => {
    const state = this.profileState().data;
    return state.kind === 'profile' ? state.profile : null;
  });
  protected profileUnavailable = computed(() => {
    const state = this.profileState().data;
    return state.kind === 'unavailable' ? state.reason : null;
  });

  protected pinned = computed(() => this.feed().watchlist.filter((w) => w.pinned));
  protected watching = computed(() => this.feed().watchlist.filter((w) => !w.pinned));
  protected watchedDeals = computed(() => this.feed().deals.filter((d) => d.watched).length);

  /** A one-liner summarising what's worth looking at right now. */
  protected intro = computed(() => {
    const feed = this.feed();
    if (feed.watchlist.length === 0) {
      return "You're not watching any games yet. Hit Watch on any game page to get price-drop alerts and better picks here.";
    }
    const deals = this.watchedDeals();
    if (deals > 0) {
      return `${deals} of the games you're watching ${deals === 1 ? 'is' : 'are'} below ${deals === 1 ? 'its' : 'their'} usual price right now.`;
    }
    return `Nothing on your watchlist is on sale right now - we'll ping you the moment something drops.`;
  });
}
