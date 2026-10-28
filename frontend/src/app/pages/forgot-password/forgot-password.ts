import { Component, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';

import { AuthService } from '../../services/auth';
import { apiErrorMessage } from '../../api';

@Component({
  selector: 'app-forgot-password',
  imports: [FormsModule, RouterLink],
  templateUrl: './forgot-password.html',
})
export class ForgotPasswordComponent {
  private auth = inject(AuthService);

  protected email = '';
  protected submitting = signal(false);
  protected error = signal<string | null>(null);
  /** The server's answer, which never says whether the email has an account. */
  protected sent = signal<string | null>(null);

  protected submit() {
    const email = this.email.trim();
    if (!email) {
      this.error.set('Enter the email address of your account.');
      return;
    }
    this.submitting.set(true);
    this.error.set(null);
    this.auth.requestPasswordReset(email).subscribe({
      next: (message) => {
        this.sent.set(message);
        this.submitting.set(false);
      },
      error: (err) => {
        this.error.set(apiErrorMessage(err, "Couldn't send a reset link."));
        this.submitting.set(false);
      },
    });
  }
}
