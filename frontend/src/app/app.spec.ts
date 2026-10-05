import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of } from 'rxjs';

import { App } from './app';
import { AuthService } from './services/auth';
import { AccountService } from './services/account';
import { User } from './models/user';
import { fakeAuth, makeUser, textOf } from '../testing/factories';

function render(user: User | null) {
  const account = {
    notifications: signal([]),
    unreadCount: () => 0,
    clearNotifications: vi.fn(),
    loadNotifications: vi.fn(),
    getPreferences: vi.fn().mockReturnValue(of({ theme: 'dark', notify_price_drops: true, price_drop_threshold_percent: 10, preferred_genres: [] })),
    updatePreferences: vi.fn().mockReturnValue(of(undefined)),
    markAllRead: vi.fn().mockReturnValue(of(undefined)),
  };
  TestBed.configureTestingModule({
    imports: [App],
    providers: [
      provideRouter([]),
      { provide: AuthService, useValue: { ...fakeAuth(user), logout: vi.fn().mockReturnValue(of(undefined)) } },
      { provide: AccountService, useValue: account },
    ],
  });
  const fixture = TestBed.createComponent(App);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const links = () => Array.from(el.querySelectorAll('a')).map((a) => [textOf(a), a.getAttribute('href')]);
  const openMenu = () => {
    (Array.from(el.querySelectorAll('button')).find((b) => textOf(b).includes('▾')) as HTMLButtonElement).click();
    fixture.detectChanges();
  };
  return { fixture, el, links, openMenu };
}

describe('App shell navigation', () => {
  it('shows Browse, Bundles and Search to everyone', () => {
    const { links } = render(null);
    expect(links()).toEqual(expect.arrayContaining([['Browse', '/games'], ['Bundles', '/bundles'], ['Search', '/search']]));
  });

  it('shows Log in / sign up to visitors and no Feed link', () => {
    const { links } = render(null);
    const text = links().map(([t]) => t);
    expect(text).not.toContain('Feed');
    expect(links().some(([, href]) => href === '/login')).toBe(true);
    expect(links().some(([, href]) => href === '/register')).toBe(true);
  });

  it('shows a Feed link and the username menu once logged in', () => {
    const { links, el } = render(makeUser());
    expect(links()).toEqual(expect.arrayContaining([['Feed', '/feed']]));
    expect(textOf(el)).toContain('alice');
    expect(links().some(([, href]) => href === '/login')).toBe(false);
  });

  it('offers account links in the menu but no Admin link to regular users', () => {
    const { links, openMenu } = render(makeUser());
    openMenu();
    const hrefs = links().map(([, href]) => href);
    expect(hrefs).toEqual(expect.arrayContaining(['/feed', '/account', '/submit']));
    expect(hrefs).not.toContain('/admin');
  });

  it('shows the Admin link in the menu for admins only', () => {
    const { links, openMenu } = render(makeUser({ is_admin: true }));
    openMenu();
    expect(links()).toEqual(expect.arrayContaining([['Admin', '/admin']]));
  });
});
