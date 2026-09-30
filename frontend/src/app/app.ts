import { Component, PLATFORM_ID, effect, inject, signal } from '@angular/core';
import { DatePipe, isPlatformBrowser } from '@angular/common';
import { NavigationEnd, Router, RouterLink, RouterOutlet } from '@angular/router';
import { filter } from 'rxjs';

import { AuthService } from './services/auth';
import { AccountService } from './services/account';
import { ThemeService } from './services/theme';
import { AppNotification } from './models/user';

const NOTIFICATION_POLL_MS = 60_000;

@Component({
  selector: 'app-root',
  imports: [RouterOutlet, RouterLink, DatePipe],
  templateUrl: './app.html',
  styleUrl: './app.css'
})
export class App {
  protected auth = inject(AuthService);
  protected account = inject(AccountService);
  protected themeService = inject(ThemeService);
  private router = inject(Router);
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  protected menuOpen = signal(false);
  protected bellOpen = signal(false);

  constructor() {
    if (!this.isBrowser) return;

    this.auth.ready();

    // While logged in: sync theme from the account's preferences and keep
    // notifications fresh. Logging out clears them.
    effect((onCleanup) => {
      const user = this.auth.user();
      if (!user) {
        this.account.clearNotifications();
        return;
      }
      this.account.getPreferences().subscribe({ next: (prefs) => this.themeService.set(prefs.theme), error: () => {} });
      this.account.loadNotifications();
      const timer = setInterval(() => this.account.loadNotifications(), NOTIFICATION_POLL_MS);
      onCleanup(() => clearInterval(timer));
    });

    this.router.events.pipe(filter((e) => e instanceof NavigationEnd)).subscribe(() => {
      this.menuOpen.set(false);
      this.bellOpen.set(false);
    });
  }

  toggleTheme() {
    const next = this.themeService.theme() === 'dark' ? 'light' : 'dark';
    this.themeService.set(next);
    if (this.auth.isLoggedIn()) {
      this.account.getPreferences().subscribe({
        next: (prefs) => this.account.updatePreferences({ ...prefs, theme: next }).subscribe(),
        error: () => {},
      });
    }
  }

  protected toggleBell() {
    this.menuOpen.set(false);
    this.bellOpen.update((open) => !open);
  }

  protected toggleMenu() {
    this.bellOpen.set(false);
    this.menuOpen.update((open) => !open);
  }

  protected markAllRead() {
    this.account.markAllRead().subscribe();
  }

  protected openNotification(n: AppNotification) {
    this.bellOpen.set(false);
    if (n.app_id) {
      this.router.navigate(['/games', n.app_id]);
    }
  }

  protected logout() {
    this.menuOpen.set(false);
    this.auth.logout().subscribe(() => this.router.navigateByUrl('/'));
  }
}
