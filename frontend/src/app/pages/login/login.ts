import { Component, inject, signal } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';

import { AuthService } from '../../services/auth';
import { apiErrorMessage } from '../../api';

const STEAM_MESSAGES: Record<string, string> = {
  notlinked: "That Steam account isn't linked to a Steamscope account yet. Log in, then link Steam from your account page.",
  cancelled: 'Steam sign-in was cancelled.',
  expired: 'That sign-in attempt expired. Please try again.',
  error: "Couldn't sign in with Steam. Please try again.",
};

/** Only allow redirecting back to paths on this site. */
function safeReturnUrl(value: string | null): string {
  return value && value.startsWith('/') && !value.startsWith('//') ? value : '/feed';
}

@Component({
  selector: 'app-login',
  imports: [FormsModule, RouterLink],
  templateUrl: './login.html',
})
export class LoginComponent {
  private auth = inject(AuthService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);

  protected login = '';
  protected password = '';
  protected submitting = signal(false);
  protected error = signal<string | null>(STEAM_MESSAGES[this.route.snapshot.queryParamMap.get('steam') ?? ''] ?? null);
  protected steamLoginUrl = this.auth.steamLoginUrl;

  protected submit() {
    if (!this.login || !this.password) {
      this.error.set('Enter your username or email and password.');
      return;
    }
    this.submitting.set(true);
    this.error.set(null);
    this.auth.login(this.login, this.password).subscribe({
      next: () => this.router.navigateByUrl(safeReturnUrl(this.route.snapshot.queryParamMap.get('returnUrl'))),
      error: (err) => {
        this.error.set(apiErrorMessage(err, 'Login failed.'));
        this.submitting.set(false);
      },
    });
  }
}
