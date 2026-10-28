import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { of, throwError } from 'rxjs';

import { AdminAuditComponent, describeAudit } from './admin-audit';
import { AdminService } from '../../../services/admin';
import { AuditEntry } from '../../../models/admin';
import { textOf } from '../../../../testing/factories';

function entry(over: Partial<AuditEntry> = {}): AuditEntry {
  return {
    audit_id: 1, admin_id: 6, admin_username: 'root', action: 'user.ban', target_type: 'user', target_id: '42',
    details: { username: 'spammer' }, ip: '10.0.0.1', created_at: '2026-10-20T12:00:00Z', ...over,
  };
}

function render(getAuditLog = vi.fn().mockReturnValue(of([entry()]))) {
  TestBed.configureTestingModule({
    imports: [AdminAuditComponent],
    providers: [provideRouter([]), { provide: AdminService, useValue: { getAuditLog } }],
  });
  const fixture = TestBed.createComponent(AdminAuditComponent);
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement, getAuditLog };
}

describe('describeAudit', () => {
  it.each<[Partial<AuditEntry>, string]>([
    [{}, 'Banned spammer'],
    [{ action: 'user.unban' }, 'Unbanned spammer'],
    [{ action: 'user.block_submissions' }, 'Blocked submissions from spammer'],
    [{ action: 'user.unblock_submissions' }, 'Allowed submissions from spammer again'],
    [{ action: 'user.delete', details: { username: 'bob', email: 'bob@example.com' } }, 'Deleted bob (bob@example.com)'],
    [{ action: 'user.ban', details: {} }, 'Banned user #42'],
    [{ action: 'item.delete', target_type: 'game', target_id: '730', details: { name: 'Counter-Strike 2' } }, 'Deleted "Counter-Strike 2" (game 730)'],
    [{ action: 'item.delete', target_type: 'bundle', target_id: '9', details: { name: '' } }, 'Deleted bundle 9'],
    [{ action: 'item.approve', target_type: 'game', target_id: '10' }, 'Approved the submission of game 10'],
    [{ action: 'item.reject', target_type: 'bundle', target_id: '11' }, 'Rejected the submission of bundle 11'],
    [{ action: 'blacklist.add', target_type: 'blacklist_rule', details: { field: 'developer', pattern: '^spam', note: 'junk' } }, 'Added a rule: developer matches "^spam" (junk)'],
    [{ action: 'blacklist.delete', target_type: 'blacklist_rule', details: { field: 'app_id', pattern: '123' } }, 'Removed the rule: App ID matches "123"'],
    [
      { action: 'blacklist.purge', target_type: 'blacklist_rule', details: { field: 'name', pattern: 'x', removed: 2, games: [{ name: 'A' }, { name: 'B' }] } },
      'Removed 2 games matching name matches "x": A, B',
    ],
    [{ action: 'blacklist.purge', target_type: 'blacklist_rule', target_id: '5', details: { removed: 1, games: [] } }, 'Removed 1 game matching rule #5'],
  ])('describes %o', (over, text) => {
    expect(describeAudit(entry(over))).toBe(text);
  });

  it('copes with details of the wrong shape', () => {
    expect(describeAudit(entry({ action: 'blacklist.purge', details: { games: 'nope', pattern: 7 } as Record<string, unknown> }))).toBe('Removed 0 games matching rule #42');
  });
});

describe('AdminAuditComponent', () => {
  it('lists entries with who, when, what and from where', () => {
    const { el, getAuditLog } = render();
    expect(getAuditLog).toHaveBeenCalledWith({ action: '', before: undefined, limit: 50 });
    const row = el.querySelector('.audit-entry')!;
    expect(textOf(row.querySelector('.audit-action'))).toBe('Ban');
    expect(textOf(row.querySelector('.audit-summary'))).toBe('Banned spammer');
    expect(textOf(row.querySelector('.audit-admin'))).toContain('by root');
    expect(textOf(row.querySelector('.audit-admin'))).toContain('10.0.0.1');
    expect(row.querySelector('time')?.getAttribute('datetime')).toBe('2026-10-20T12:00:00Z');
  });

  it('notes when the admin account has since been removed', () => {
    const { el } = render(vi.fn().mockReturnValue(of([entry({ admin_id: null })])));
    expect(textOf(el.querySelector('.audit-admin'))).toContain('(account removed)');
  });

  it('links to games and bundles that still exist', () => {
    const { el } = render(vi.fn().mockReturnValue(of([
      entry({ audit_id: 2, action: 'item.approve', target_type: 'game', target_id: '730' }),
      entry({ audit_id: 1, action: 'item.delete', target_type: 'game', target_id: '731' }),
    ])));
    const links = Array.from(el.querySelectorAll('.audit-summary a')).map((a) => a.getAttribute('href'));
    expect(links).toEqual(['/games/730']);
  });

  it('filters by action', async () => {
    const { el, fixture, getAuditLog } = render();
    await fixture.whenStable();
    const select = el.querySelector('select')!;
    select.value = 'user.delete';
    select.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    expect(getAuditLog).toHaveBeenLastCalledWith({ action: 'user.delete', before: undefined, limit: 50 });
  });

  it('pages back from the oldest entry shown when there may be more', () => {
    const page = Array.from({ length: 50 }, (_, i) => entry({ audit_id: 100 - i }));
    const getAuditLog = vi.fn().mockReturnValueOnce(of(page)).mockReturnValueOnce(of([entry({ audit_id: 10 })]));
    const { el, fixture } = render(getAuditLog);
    (el.querySelector('.load-more') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(getAuditLog).toHaveBeenLastCalledWith({ action: '', before: 51, limit: 50 });
    expect(el.querySelectorAll('.audit-entry')).toHaveLength(51);
    expect(el.querySelector('.load-more')).toBeNull();
  });

  it('has no load-more button on a short page', () => {
    expect(render().el.querySelector('.load-more')).toBeNull();
  });

  it('says when there is nothing yet', () => {
    const { el } = render(vi.fn().mockReturnValue(of([])));
    expect(textOf(el.querySelector('.no-entries'))).toBe('No admin actions recorded yet.');
  });

  it('shows an error when the log does not load', () => {
    const { el } = render(vi.fn().mockReturnValue(throwError(() => new HttpErrorResponse({ status: 500 }))));
    expect(textOf(el.querySelector('.error-message'))).toBeTruthy();
    expect(el.querySelector('.skeleton')).toBeNull();
  });
});
