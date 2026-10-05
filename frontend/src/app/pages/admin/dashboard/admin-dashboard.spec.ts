import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { HttpErrorResponse } from '@angular/common/http';

import { AdminDashboardComponent } from './admin-dashboard';
import { AdminService } from '../../../services/admin';
import { AdminStats } from '../../../models/admin';
import { makeAdminUser, textOf } from '../../../../testing/factories';

function makeStats(over: Partial<AdminStats> = {}): AdminStats {
  const day = (n: number, signups: number, watch: number, subs: number) => ({
    date: `2026-10-${String(n).padStart(2, '0')}T00:00:00Z`, signups, watch_adds: watch, submissions: subs,
  });
  return {
    users: 42, new_users_7d: 5, banned_users: 1, tracked_games: 17, tracked_bundles: 14, awaiting_approval: 3,
    watchlist_entries: 120, pinned_entries: 30, blacklist_rules: 2, users_with_watchlist: 25,
    most_watched: [{ app_id: 730, name: 'Counter-Strike 2', count: 10 }, { app_id: 570, name: 'Dota 2', count: 5 }],
    most_pinned: [{ app_id: 730, name: 'Counter-Strike 2', count: 4 }],
    top_submitters: [{ username: 'alice', count: 7 }],
    top_watchers: [{ username: 'bob', count: 12 }],
    activity: [day(1, 0, 0, 0), day(2, 4, 2, 1), day(3, 2, 8, 0)],
    recent_users: [makeAdminUser({ username: 'newest' })],
    recent_items: [
      { kind: 'app', id: 1, name: 'Some Game', status: 'awaiting_approval', submitted_by: 'alice', created_at: '2026-10-03T00:00:00Z' },
      { kind: 'bundle', id: 9, name: null, status: 'tracked', submitted_by: null, created_at: '2026-10-02T00:00:00Z' },
    ],
    ...over,
  };
}

function render(getStats: () => unknown) {
  TestBed.configureTestingModule({
    imports: [AdminDashboardComponent],
    providers: [provideRouter([]), { provide: AdminService, useValue: { getStats } }],
  });
  const fixture = TestBed.createComponent(AdminDashboardComponent);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

const section = (el: HTMLElement, heading: string) =>
  Array.from(el.querySelectorAll('section')).find((s) => textOf(s.querySelector('h2')) === heading)!;

describe('AdminDashboardComponent', () => {
  it('shows the headline stat cards with their sub-labels', () => {
    const el = render(() => of(makeStats()));
    const text = textOf(el);
    for (const expected of ['Users 42 5 new this week', 'Tracked games 17 14 bundles', 'Awaiting approval 3', 'Watchlist entries 120 25 users, 30 pinned', 'Banned users 1', 'Blacklist rules 2']) {
      expect(text, expected).toContain(expected);
    }
  });

  it('highlights awaiting approvals only when there are some, linking to the queue', () => {
    const withWork = render(() => of(makeStats()));
    const card = Array.from(withWork.querySelectorAll('.edge-panel')).find((c) => textOf(c).startsWith('Awaiting approval'))!;
    expect((card as HTMLElement).style.borderColor).toContain('var(--color-accent)');
    expect(card.querySelector('a')?.getAttribute('href')).toBe('/admin/games');
  });

  it('does not highlight the approvals card when the queue is empty', () => {
    const el = render(() => of(makeStats({ awaiting_approval: 0 })));
    const card = Array.from(el.querySelectorAll('.edge-panel')).find((c) => textOf(c).startsWith('Awaiting approval')) as HTMLElement;
    expect(card.style.borderColor).toBe('');
  });

  it('ranks the most watched and most pinned games, linking to each', () => {
    const el = render(() => of(makeStats()));
    const watched = Array.from(section(el, 'Most watched').querySelectorAll('li'));
    expect(watched.map((l) => textOf(l))).toEqual(['Counter-Strike 2 10', 'Dota 2 5']);
    expect(watched[0].querySelector('a')?.getAttribute('href')).toBe('/games/730');
    expect(textOf(section(el, 'Most pinned'))).toContain('Counter-Strike 2 4');
  });

  it('scales ranking bars relative to the top entry', () => {
    const el = render(() => of(makeStats()));
    const bars = Array.from(section(el, 'Most watched').querySelectorAll<HTMLElement>('li > div.absolute'));
    expect(bars.map((b) => b.style.width)).toEqual(['100%', '50%']);
  });

  it('lists top watchers, top submitters, newest users and latest submissions', () => {
    const el = render(() => of(makeStats()));
    expect(textOf(section(el, 'Biggest watchlists'))).toContain('bob 12');
    expect(textOf(section(el, 'Top submitters'))).toContain('alice 7');
    expect(textOf(section(el, 'Newest users'))).toContain('newest');
    const latest = textOf(section(el, 'Latest submissions'));
    expect(latest).toContain('Some Game · awaiting approval');
    expect(latest).toContain('Bundle 9 · tracked'); // unnamed items fall back to kind + id
  });

  it('draws one bar per day for each activity series, with totals', () => {
    const el = render(() => of(makeStats()));
    const panels = Array.from(el.querySelectorAll('.grid.md\\:grid-cols-3 > .edge-panel'));
    expect(panels).toHaveLength(3);
    expect(panels.map((p) => textOf(p.querySelector('span:last-child')))).toEqual(['6', '10', '1']);
    for (const p of panels) expect(p.querySelectorAll('.items-end > div')).toHaveLength(3);
  });

  it('gives zero days a minimal faded bar so the chart still reads', () => {
    const el = render(() => of(makeStats()));
    const firstBar = el.querySelector<HTMLElement>('.items-end > div')!;
    expect(firstBar.style.height).toBe('2%');
    expect(firstBar.style.opacity).toBe('0.25');
  });

  it('shows friendly empty states for every ranked list', () => {
    const el = render(() => of(makeStats({ most_watched: [], most_pinned: [], top_submitters: [], top_watchers: [], recent_items: [] })));
    const text = textOf(el);
    for (const msg of ['No one is watching anything yet.', 'Nothing is pinned yet.', 'No watchlists yet.', 'No submissions yet.']) {
      expect(text, msg).toContain(msg);
    }
  });

  it('shows a skeleton until the stats arrive', () => {
    const el = render(() => new (class { subscribe() {} })());
    expect(el.querySelector('.skeleton')).not.toBeNull();
  });

  it('shows the error and no skeleton when loading fails', () => {
    const el = render(() => throwError(() => new HttpErrorResponse({ status: 500, error: { error: 'failed to load stats' } })));
    expect(textOf(el)).toContain('Failed to load stats');
    expect(el.querySelector('.skeleton')).toBeNull();
  });
});
