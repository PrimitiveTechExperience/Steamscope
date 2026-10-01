import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';

import { AdminService } from '../../../services/admin';
import { AuthService } from '../../../services/auth';
import { AdminUser } from '../../../models/admin';
import { apiErrorMessage } from '../../../api';

@Component({
  selector: 'app-admin-users',
  imports: [DatePipe],
  templateUrl: './admin-users.html',
})
export class AdminUsersComponent implements OnInit {
  private admin = inject(AdminService);
  private auth = inject(AuthService);

  protected users = signal<AdminUser[] | null>(null);
  protected error = signal<string | null>(null);
  protected search = signal('');

  protected filtered = computed(() => {
    const term = this.search().trim().toLowerCase();
    return (this.users() ?? []).filter((u) => !term || `${u.username} ${u.email}`.toLowerCase().includes(term));
  });

  ngOnInit() {
    this.load();
  }

  protected onSearch(event: Event) {
    this.search.set((event.target as HTMLInputElement).value);
  }

  /** Admins (including you) can't be banned, blocked or deleted from here. */
  protected canModerate(user: AdminUser): boolean {
    return !user.is_admin && user.user_id !== this.auth.user()?.user_id;
  }

  protected toggleBan(user: AdminUser) {
    const ban = !user.is_banned;
    const question = ban
      ? `Ban ${user.username}? They'll be logged out and unable to sign in.`
      : `Unban ${user.username}?`;
    if (!confirm(question)) return;
    this.run(this.admin.moderateUser(user.user_id, { is_banned: ban }), "Couldn't update that user.");
  }

  protected toggleSubmissions(user: AdminUser) {
    this.run(
      this.admin.moderateUser(user.user_id, { submissions_blocked: !user.submissions_blocked }),
      "Couldn't update that user."
    );
  }

  protected deleteUser(user: AdminUser) {
    if (!confirm(`Delete user ${user.username}? Their watchlist, preferences and notifications are removed too.`)) return;
    this.run(this.admin.deleteUser(user.user_id), "Couldn't delete that user.");
  }

  private run(request: ReturnType<AdminService['deleteUser']>, fallback: string) {
    this.error.set(null);
    request.subscribe({
      next: () => this.load(),
      error: (err) => this.error.set(apiErrorMessage(err, fallback)),
    });
  }

  private load() {
    this.admin.getUsers().subscribe({
      next: (users) => this.users.set(users),
      error: (err) => this.error.set(apiErrorMessage(err, "Couldn't load users.")),
    });
  }
}
