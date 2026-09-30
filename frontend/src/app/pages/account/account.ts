import { Component, OnInit, inject, signal } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { toSignal } from '@angular/core/rxjs-interop';
import { catchError, map, of } from 'rxjs';

import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { GamesService } from '../../services/games';
import { ThemeService } from '../../services/theme';
import { FilterAutocompleteComponent } from '../../components/filter-autocomplete/filter-autocomplete';
import { Preferences } from '../../models/user';
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

  protected prefs = signal<Preferences | null>(null);
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
