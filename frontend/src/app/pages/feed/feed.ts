import { Component, computed, inject } from '@angular/core';
import { RouterLink } from '@angular/router';
import { CurrencyPipe, DecimalPipe } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { toSignal } from '@angular/core/rxjs-interop';
import { catchError, map, of } from 'rxjs';

import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { GameCardComponent } from '../../components/game-card/game-card';
import { RevealOnScrollDirective } from '../../directives/reveal-on-scroll';
import { Feed, SteamProfile } from '../../models/user';
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
  imports: [RouterLink, CurrencyPipe, DecimalPipe, GameCardComponent, RevealOnScrollDirective],
  templateUrl: './feed.html',
})
export class FeedComponent {
  protected auth = inject(AuthService);
  private account = inject(AccountService);

  protected greeting = greetingFor(new Date().getHours());

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
