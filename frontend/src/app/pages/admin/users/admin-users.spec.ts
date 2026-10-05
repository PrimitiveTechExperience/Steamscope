import { TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { HttpErrorResponse } from '@angular/common/http';

import { AdminUsersComponent } from './admin-users';
import { AdminService } from '../../../services/admin';
import { AuthService } from '../../../services/auth';
import { AdminUser } from '../../../models/admin';
import { fakeAuth, makeAdminUser, makeUser, textOf } from '../../../../testing/factories';

const USERS: AdminUser[] = [
  makeAdminUser({ user_id: 1, username: 'root', email: 'root@example.com', is_admin: true }),
  makeAdminUser({ user_id: 2, username: 'bob', email: 'bob@example.com' }),
  makeAdminUser({ user_id: 3, username: 'mallory', email: 'mal@example.com', is_banned: true }),
  makeAdminUser({ user_id: 4, username: 'spammer', email: 'spam@example.com', submissions_blocked: true }),
];

function render(admin: Partial<Record<keyof AdminService, unknown>> = {}) {
  const service = {
    getUsers: vi.fn().mockReturnValue(of(USERS)),
    moderateUser: vi.fn().mockReturnValue(of(undefined)),
    deleteUser: vi.fn().mockReturnValue(of(undefined)),
    ...admin,
  };
  TestBed.configureTestingModule({
    imports: [AdminUsersComponent],
    providers: [
      { provide: AdminService, useValue: service },
      { provide: AuthService, useValue: fakeAuth(makeUser({ user_id: 1, username: 'root', is_admin: true })) },
    ],
  });
  const fixture = TestBed.createComponent(AdminUsersComponent);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const rows = () => Array.from(el.querySelectorAll('li'));
  const row = (name: string) => rows().find((r) => textOf(r).startsWith(name))!;
  const button = (r: Element, label: string) => Array.from(r.querySelectorAll('button')).find((b) => textOf(b) === label) as HTMLButtonElement | undefined;
  return { fixture, el, service, rows, row, button };
}

describe('AdminUsersComponent', () => {
  beforeEach(() => vi.stubGlobal('confirm', vi.fn().mockReturnValue(true)));
  afterEach(() => vi.unstubAllGlobals());

  it('lists every user with their email and joined date', () => {
    const { rows, row } = render();
    expect(rows()).toHaveLength(4);
    expect(textOf(row('bob'))).toContain('bob@example.com');
    expect(textOf(row('bob'))).toContain('joined Feb 1, 2026');
    expect(textOf(document.body.querySelector('h2'))).toBe('Users (4)');
  });

  it('badges admins, banned users and submission-blocked users', () => {
    const { row } = render();
    expect(textOf(row('root'))).toContain('admin');
    expect(textOf(row('mallory'))).toContain('banned');
    expect(textOf(row('spammer'))).toContain("can't submit");
    expect(textOf(row('bob'))).not.toMatch(/admin|banned|can't submit/);
  });

  it('offers no moderation buttons for admins or for yourself', () => {
    const { row } = render();
    expect(row('root').querySelectorAll('button')).toHaveLength(0);
    expect(row('bob').querySelectorAll('button')).toHaveLength(3);
  });

  it('labels the toggles by current state', () => {
    const { row, button } = render();
    expect(button(row('bob'), 'Ban')).toBeDefined();
    expect(button(row('bob'), 'Block submissions')).toBeDefined();
    expect(button(row('mallory'), 'Unban')).toBeDefined();
    expect(button(row('spammer'), 'Allow submissions')).toBeDefined();
  });

  it('filters by username or email as you type', () => {
    const { fixture, el, rows } = render();
    const input = el.querySelector('input[type="search"]') as HTMLInputElement;
    input.value = 'MAL@';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(rows().map((r) => textOf(r).split(' ')[0])).toEqual(['mallory']);

    input.value = 'nobody-matches';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(textOf(el)).toContain('No users match.');
  });

  it('bans after confirmation and reloads the list', () => {
    const { fixture, service, row, button } = render();
    button(row('bob'), 'Ban')!.click();
    expect(service.moderateUser).toHaveBeenCalledWith(2, { is_banned: true });
    expect(service.getUsers).toHaveBeenCalledTimes(2);
    fixture.detectChanges();
  });

  it('does nothing when the confirmation is declined', () => {
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false));
    const { service, row, button } = render();
    button(row('bob'), 'Ban')!.click();
    button(row('bob'), 'Delete')!.click();
    expect(service.moderateUser).not.toHaveBeenCalled();
    expect(service.deleteUser).not.toHaveBeenCalled();
  });

  it('unbans and toggles submission blocking', () => {
    const { service, row, button } = render();
    button(row('mallory'), 'Unban')!.click();
    expect(service.moderateUser).toHaveBeenCalledWith(3, { is_banned: false });
    button(row('spammer'), 'Allow submissions')!.click();
    expect(service.moderateUser).toHaveBeenCalledWith(4, { submissions_blocked: false });
    button(row('bob'), 'Block submissions')!.click();
    expect(service.moderateUser).toHaveBeenCalledWith(2, { submissions_blocked: true });
  });

  it('deletes a user after confirmation', () => {
    const { service, row, button } = render();
    button(row('bob'), 'Delete')!.click();
    expect(service.deleteUser).toHaveBeenCalledWith(2);
  });

  it('shows the server error when an action fails', () => {
    const { fixture, el, row, button } = render({
      moderateUser: vi.fn().mockReturnValue(throwError(() => new HttpErrorResponse({ status: 404, error: { error: 'no such user (admins can\'t be moderated)' } }))),
    });
    button(row('bob'), 'Ban')!.click();
    fixture.detectChanges();
    expect(textOf(el.querySelector('p.border-2'))).toBe("No such user (admins can't be moderated)");
  });

  it('shows a load error instead of an endless skeleton', () => {
    const { el } = render({ getUsers: vi.fn().mockReturnValue(throwError(() => new HttpErrorResponse({ status: 403, error: { error: 'admin access required' } }))) });
    expect(textOf(el)).toContain('Admin access required');
  });
});
