import { Component, inject, signal } from '@angular/core';
import { Location } from '@angular/common';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';

import { AuthService } from '../../services/auth';
import { apiErrorMessage } from '../../api';

@Component({
  selector: 'app-reset-password',
  imports: [FormsModule, RouterLink],
  templateUrl: './reset-password.html',
})
export class ResetPasswordComponent {
  private auth = inject(AuthService);
  private router = inject(Router);

  /**
   * The token from the emailed link. It is taken out of the address bar
   * straight away, so it doesn't stay in the browser history or get shared
   * along with the page's address.
   */
  protected readonly token = inject(ActivatedRoute).snapshot.queryParamMap.get('token') ?? '';

  protected password = '';
  protected confirm = '';
  protected submitting = signal(false);
  protected error = signal<string | null>(null);

  constructor() {
    if (this.token) inject(Location).replaceState('/reset-password');
  }

  protected submit() {
    if (this.password.length < 8 || this.password.length > 128) {
      this.error.set('Your new password must be 8-128 characters.');
      return;
    }
    if (this.password !== this.confirm) {
      this.error.set("The two passwords don't match.");
      return;
    }
    this.submitting.set(true);
    this.error.set(null);
    this.auth.confirmPasswordReset(this.token, this.password).subscribe({
      next: () => this.router.navigate(['/login'], { queryParams: { reset: 'done' } }),
      error: (err) => {
        this.error.set(apiErrorMessage(err, "Couldn't change your password."));
        this.submitting.set(false);
      },
    });
  }
}
