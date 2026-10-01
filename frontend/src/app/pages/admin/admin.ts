import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { AdminService } from '../../services/admin';
import { AuthService } from '../../services/auth';
import { AdminItem, AdminUser } from '../../models/admin';
import { apiErrorMessage } from '../../api';

@Component({
  selector: 'app-admin',
  imports: [DatePipe, RouterLink],
  templateUrl: './admin.html',
})
export class AdminComponent implements OnInit {
  private admin = inject(AdminService);
  protected auth = inject(AuthService);

  protected items = signal<AdminItem[] | null>(null);
  protected users = signal<AdminUser[] | null>(null);
  protected error = signal<string | null>(null);

  protected awaiting = computed(() => (this.items() ?? []).filter((i) => i.status === 'awaiting_approval'));
  protected others = computed(() => (this.items() ?? []).filter((i) => i.status !== 'awaiting_approval'));

  ngOnInit() {
    this.loadItems();
    this.loadUsers();
  }

  protected storeUrl(item: AdminItem): string {
    return `https://store.steampowered.com/${item.kind}/${item.id}`;
  }

  protected label(item: AdminItem): string {
    return item.name || `${item.kind === 'bundle' ? 'Bundle' : 'App'} ${item.id}`;
  }

  protected approve(item: AdminItem) {
    this.run(this.admin.approveItem(item), "Couldn't approve that submission.", () => this.loadItems());
  }

  protected reject(item: AdminItem) {
    this.run(this.admin.rejectItem(item), "Couldn't reject that submission.", () => this.loadItems());
  }

  protected deleteItem(item: AdminItem) {
    if (!confirm(`Delete ${this.label(item)} and all of its stored data? This can't be undone.`)) return;
    this.run(this.admin.deleteItem(item), "Couldn't delete that.", () => this.loadItems());
  }

  protected deleteUser(user: AdminUser) {
    if (!confirm(`Delete user ${user.username}? Their watchlist, preferences and notifications are removed too.`)) return;
    this.run(this.admin.deleteUser(user.user_id), "Couldn't delete that user.", () => this.loadUsers());
  }

  protected canDelete(user: AdminUser): boolean {
    return !user.is_admin && user.user_id !== this.auth.user()?.user_id;
  }

  private run(request: ReturnType<AdminService['approveItem']>, fallback: string, done: () => void) {
    this.error.set(null);
    request.subscribe({
      next: done,
      error: (err) => this.error.set(apiErrorMessage(err, fallback)),
    });
  }

  private loadItems() {
    this.admin.getItems().subscribe({
      next: (items) => this.items.set(items),
      error: (err) => this.error.set(apiErrorMessage(err, "Couldn't load games.")),
    });
  }

  private loadUsers() {
    this.admin.getUsers().subscribe({
      next: (users) => this.users.set(users),
      error: (err) => this.error.set(apiErrorMessage(err, "Couldn't load users.")),
    });
  }
}
