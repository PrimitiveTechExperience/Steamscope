import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { AdminService } from '../../../services/admin';
import { AdminStats, DayActivity, GameCount, UserCount } from '../../../models/admin';
import { apiErrorMessage } from '../../../api';

interface Series {
  label: string;
  key: 'signups' | 'watch_adds' | 'submissions';
}

@Component({
  selector: 'app-admin-dashboard',
  imports: [DatePipe, RouterLink],
  templateUrl: './admin-dashboard.html',
})
export class AdminDashboardComponent implements OnInit {
  private admin = inject(AdminService);

  protected stats = signal<AdminStats | null>(null);
  protected error = signal<string | null>(null);

  protected series: Series[] = [
    { label: 'Sign-ups', key: 'signups' },
    { label: 'Watchlist adds', key: 'watch_adds' },
    { label: 'Submissions', key: 'submissions' },
  ];

  protected cards = computed(() => {
    const s = this.stats();
    if (!s) return [];
    return [
      { label: 'Users', value: s.users, sub: `${s.new_users_7d} new this week` },
      { label: 'Tracked games', value: s.tracked_games, sub: `${s.tracked_bundles} bundles` },
      { label: 'Awaiting approval', value: s.awaiting_approval, sub: 'submissions', link: '/admin/games', highlight: s.awaiting_approval > 0 },
      { label: 'Watchlist entries', value: s.watchlist_entries, sub: `${s.users_with_watchlist} users, ${s.pinned_entries} pinned` },
      { label: 'Banned users', value: s.banned_users, sub: 'suspended accounts', link: '/admin/users' },
      { label: 'Blacklist rules', value: s.blacklist_rules, sub: 'keep games off the site', link: '/admin/blacklist' },
    ];
  });

  ngOnInit() {
    this.admin.getStats().subscribe({
      next: (s) => this.stats.set(s),
      error: (err) => this.error.set(apiErrorMessage(err, "Couldn't load the dashboard.")),
    });
  }

  /** Bar height (percent) for one day relative to the series' busiest day. */
  protected barHeight(days: DayActivity[], key: Series['key'], day: DayActivity): number {
    const max = Math.max(1, ...days.map((d) => d[key]));
    return day[key] === 0 ? 2 : Math.max(8, (day[key] / max) * 100);
  }

  protected total(days: DayActivity[], key: Series['key']): number {
    return days.reduce((sum, d) => sum + d[key], 0);
  }

  /** Width (percent) of a ranked row relative to the top row. */
  protected share(rows: (GameCount | UserCount)[], row: GameCount | UserCount): number {
    const max = Math.max(1, ...rows.map((r) => r.count));
    return (row.count / max) * 100;
  }
}
