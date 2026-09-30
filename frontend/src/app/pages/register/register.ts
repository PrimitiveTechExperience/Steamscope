import { Component, inject, signal } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';

import { AuthService } from '../../services/auth';
import { apiErrorMessage } from '../../api';

const USERNAME_FORMAT = /^[A-Za-z0-9_-]{3,20}$/;

@Component({
  selector: 'app-register',
  imports: [FormsModule, RouterLink],
  templateUrl: './register.html',
})
export class RegisterComponent {
  private auth = inject(AuthService);
  private router = inject(Router);

  protected username = '';
  protected email = '';
  protected password = '';
  protected confirmPassword = '';
  protected submitting = signal(false);
  protected error = signal<string | null>(null);

  protected submit() {
    // Mirrors the server's checks for fast feedback; the server re-validates
    // everything (including the offensive-name filter, which lives there only).
    if (!USERNAME_FORMAT.test(this.username)) {
      this.error.set('Username must be 3-20 characters: letters, numbers, _ or -.');
      return;
    }
    if (!this.email.includes('@')) {
      this.error.set('Please enter a valid email address.');
      return;
    }
    if (this.password.length < 8) {
      this.error.set('Password must be at least 8 characters.');
      return;
    }
    if (this.password !== this.confirmPassword) {
      this.error.set("Passwords don't match.");
      return;
    }

    this.submitting.set(true);
    this.error.set(null);
    this.auth.register(this.username, this.email, this.password).subscribe({
      next: () => this.router.navigateByUrl('/feed'),
      error: (err) => {
        this.error.set(apiErrorMessage(err, 'Sign-up failed.'));
        this.submitting.set(false);
      },
    });
  }
}
