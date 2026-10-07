import { Component, OnInit, PLATFORM_ID, inject, signal } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { DatePipe, isPlatformBrowser } from '@angular/common';
import { toSignal } from '@angular/core/rxjs-interop';
import { catchError, map, of } from 'rxjs';

import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { GamesService } from '../../services/games';
import { ThemeService } from '../../services/theme';
import { FilterAutocompleteComponent } from '../../components/filter-autocomplete/filter-autocomplete';
import { AlertChannel, AlertChannels, Preferences } from '../../models/user';
import { PushError, PushService } from '../../services/push';
import { apiErrorMessage } from '../../api';

const STEAM_MESSAGES: Record<string, { kind: 'ok' | 'error'; text: string }> = {
  linked: { kind: 'ok', text: 'Steam account linked.' },
  taken: { kind: 'error', text: 'That Steam account is already linked to a different Steamscope account.' },
  cancelled: { kind: 'error', text: 'Steam linking was cancelled.' },
  expired: { kind: 'error', text: 'That linking attempt expired. Please try again.' },
  error: { kind: 'error', text: "Couldn't link Steam. Please try again." },
};

@Component({
  selector: 'app-account',
  imports: [FormsModule, RouterLink, DatePipe, FilterAutocompleteComponent],
  templateUrl: './account.html',
})
export class AccountComponent implements OnInit {
  protected auth = inject(AuthService);
  private account = inject(AccountService);
  private theme = inject(ThemeService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);

  private push = inject(PushService);
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  protected prefs = signal<Preferences | null>(null);
  /** Which alert channels the server can send through; null until it answers. */
  protected channels = signal<AlertChannels | null>(null);
  /** Whether this browser is subscribed to push, so the box can say so. */
  protected pushHere = signal(false);
  protected pushBusy = signal(false);
  /** The channel a test alert is being sent through, and how the last one went. */
  protected testing = signal<AlertChannel | null>(null);
  protected testResult = signal<{ channel: AlertChannel; kind: 'ok' | 'error'; text: string } | null>(null);
  protected pushMessage = signal<string | null>(null);
  protected saving = signal(false);
  protected prefsMessage = signal<{ kind: 'ok' | 'error'; text: string } | null>(null);
  protected steamMessage = signal(STEAM_MESSAGES[this.route.snapshot.queryParamMap.get('steam') ?? ''] ?? null);

  protected genreOptions = toSignal(
    inject(GamesService).getFilterOptions().pipe(
      map((options) => options.genres),
      catchError(() => of([] as string[]))
    ),
    { initialValue: [] as string[] }
  );

  ngOnInit() {
    // Coming back from the Steam round trip: re-read the user so the
    // linked Steam ID shows up, then drop the query param from the URL.
    if (this.route.snapshot.queryParamMap.has('steam')) {
      this.auth.refresh();
      this.router.navigate([], { queryParams: {}, replaceUrl: true });
    }
    this.account.getPreferences().subscribe({
      next: (prefs) => this.prefs.set(prefs),
      error: () => this.prefsMessage.set({ kind: 'error', text: "Couldn't load your preferences." }),
    });
    this.account.getAlertChannels().subscribe({
      next: (channels) => this.channels.set(channels),
      // If we cannot tell, offer only what needs nothing from the server.
      error: () => this.channels.set({ email: false, discord: true, push: false, vapid_public_key: '' }),
    });
    if (this.isBrowser) this.push.current().then((sub) => this.pushHere.set(sub !== null));
  }

  /** Whether the browser can do push at all, and has not been told no. */
  protected get pushUsable(): boolean {
    return this.push.supported && !this.push.blocked;
  }

  protected pushUnavailableReason(): string | null {
    const c = this.channels();
    if (c && !c.push) return 'Not set up on this server.';
    if (!this.push.supported) return "This browser doesn't support push notifications.";
    if (this.push.blocked) return 'Notifications are blocked for this site in your browser settings.';
    return null;
  }

  /** Ticking "Browser notification" asks the browser for permission and subscribes it. */
  protected async togglePush(event: Event) {
    const input = event.target as HTMLInputElement;
    this.pushMessage.set(null);
    this.testResult.set(null);
    if (!input.checked) {
      this.updatePrefs({ alert_push: false });
      this.pushBusy.set(true);
      const endpoint = await this.push.disable();
      if (endpoint) this.account.deletePushSubscription(endpoint).subscribe({ error: () => {} });
      this.pushHere.set(false);
      this.pushBusy.set(false);
      return;
    }
    await this.subscribeThisBrowser(input);
  }

  /** Subscribes this browser and, on success, ticks the box. */
  protected async subscribeThisBrowser(input?: HTMLInputElement) {
    this.pushBusy.set(true);
    this.pushMessage.set(null);
    try {
      const subscription = await this.push.enable(this.channels()?.vapid_public_key ?? '');
      await new Promise<void>((resolve, reject) =>
        this.account.savePushSubscription(subscription).subscribe({ next: () => resolve(), error: reject })
      );
      this.pushHere.set(true);
      this.updatePrefs({ alert_push: true });
    } catch (err) {
      this.updatePrefs({ alert_push: false });
      // The browser already ticked the box when it was clicked. The setting it shows did
      // not change, so Angular would leave it ticked: put it back by hand.
      if (input) input.checked = false;
      this.pushMessage.set(err instanceof PushError ? err.message : apiErrorMessage(err, "Couldn't turn on push notifications."));
    } finally {
      this.pushBusy.set(false);
    }
  }

  /** Sends a sample alert through one channel. Discord tries the URL as typed, saved or not. */
  protected sendTest(channel: AlertChannel) {
    this.testing.set(channel);
    this.testResult.set(null);
    const url = channel === 'discord' ? this.prefs()?.discord_webhook_url.trim() : undefined;
    this.account.sendTestAlert(channel, url).subscribe({
      next: () => {
        this.testing.set(null);
        this.testResult.set({ channel, kind: 'ok', text: 'Sent. Check for it now.' });
      },
      error: (err) => {
        this.testing.set(null);
        this.testResult.set({ channel, kind: 'error', text: apiErrorMessage(err, "Couldn't send the test alert.") });
      },
    });
  }

  protected updatePrefs(patch: Partial<Preferences>) {
    const current = this.prefs();
    if (current) this.prefs.set({ ...current, ...patch });
  }

  protected toggleGenre(genre: string) {
    const current = this.prefs();
    if (!current) return;
    const genres = current.preferred_genres.includes(genre)
      ? current.preferred_genres.filter((g) => g !== genre)
      : [...current.preferred_genres, genre];
    this.updatePrefs({ preferred_genres: genres });
  }

  protected savePrefs() {
    const prefs = this.prefs();
    if (!prefs) return;
    this.saving.set(true);
    this.prefsMessage.set(null);
    this.account.updatePreferences(prefs).subscribe({
      next: (saved) => {
        this.saving.set(false);
        this.theme.set(saved.theme);
        this.prefsMessage.set({ kind: 'ok', text: 'Preferences saved.' });
      },
      error: (err) => {
        this.saving.set(false);
        this.prefsMessage.set({ kind: 'error', text: apiErrorMessage(err, "Couldn't save preferences.") });
      },
    });
  }

  protected unlinkSteam() {
    this.account.unlinkSteam().subscribe({
      next: () => {
        this.auth.refresh();
        this.steamMessage.set({ kind: 'ok', text: 'Steam account unlinked.' });
      },
      error: (err) => this.steamMessage.set({ kind: 'error', text: apiErrorMessage(err, "Couldn't unlink Steam.") }),
    });
  }
}
