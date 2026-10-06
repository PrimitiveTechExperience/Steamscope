import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { DatePipe, NgTemplateOutlet } from '@angular/common';
import { RouterLink } from '@angular/router';

import { AdminService } from '../../../services/admin';
import { AdminItem } from '../../../models/admin';
import { apiErrorMessage } from '../../../api';

type StatusFilter = 'all' | 'tracked' | 'failed' | 'rejected' | 'pending';

@Component({
  selector: 'app-admin-games',
  imports: [DatePipe, NgTemplateOutlet, RouterLink],
  templateUrl: './admin-games.html',
})
export class AdminGamesComponent implements OnInit {
  private admin = inject(AdminService);

  protected items = signal<AdminItem[] | null>(null);
  protected error = signal<string | null>(null);
  protected search = signal('');
  protected statusFilter = signal<StatusFilter>('all');
  protected filters: StatusFilter[] = ['all', 'tracked', 'pending', 'failed', 'rejected'];

  protected awaiting = computed(() => (this.items() ?? []).filter((i) => i.status === 'awaiting_approval'));

  /** Everything not awaiting approval that matches the search and status filter. */
  private matching = computed(() => {
    const term = this.search().trim().toLowerCase();
    const status = this.statusFilter();
    return (this.items() ?? []).filter(
      (i) =>
        i.status !== 'awaiting_approval' &&
        (status === 'all' || i.status === status) &&
        (!term || `${i.name ?? ''} ${i.id} ${i.submitted_by ?? ''}`.toLowerCase().includes(term))
    );
  });

  protected games = computed(() => this.matching().filter((i) => i.kind === 'app'));
  protected bundles = computed(() => this.matching().filter((i) => i.kind === 'bundle'));

  ngOnInit() {
    this.loadItems();
  }

  protected onSearch(event: Event) {
    this.search.set((event.target as HTMLInputElement).value);
  }

  protected storeUrl(item: AdminItem): string {
    return `https://store.steampowered.com/${item.kind}/${item.id}`;
  }

  protected label(item: AdminItem): string {
    return item.name || `${item.kind === 'bundle' ? 'Bundle' : 'App'} ${item.id}`;
  }

  protected approve(item: AdminItem) {
    this.run(this.admin.approveItem(item), "Couldn't approve that submission.");
  }

  protected reject(item: AdminItem) {
    this.run(this.admin.rejectItem(item), "Couldn't reject that submission.");
  }

  protected deleteItem(item: AdminItem) {
    if (!confirm(`Delete ${this.label(item)} and all of its stored data? This can't be undone.`)) return;
    this.run(this.admin.deleteItem(item), "Couldn't delete that.");
  }

  private run(request: ReturnType<AdminService['approveItem']>, fallback: string) {
    this.error.set(null);
    request.subscribe({
      next: () => this.loadItems(),
      error: (err) => this.error.set(apiErrorMessage(err, fallback)),
    });
  }

  private loadItems() {
    this.admin.getItems().subscribe({
      next: (items) => this.items.set(items),
      error: (err) => this.error.set(apiErrorMessage(err, "Couldn't load games.")),
    });
  }
}
