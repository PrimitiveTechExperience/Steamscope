import { TestBed } from '@angular/core/testing';
import { Router, UrlTree, provideRouter } from '@angular/router';
import { ActivatedRouteSnapshot, RouterStateSnapshot } from '@angular/router';

import { adminGuard, authGuard, guestGuard } from './auth.guard';
import { AuthService } from '../services/auth';
import { User } from '../models/user';
import { fakeAuth, makeUser } from '../../testing/factories';

function run(guard: typeof authGuard, user: User | null, url = '/feed') {
  TestBed.configureTestingModule({ providers: [provideRouter([]), { provide: AuthService, useValue: fakeAuth(user) }] });
  return TestBed.runInInjectionContext(() => guard({} as ActivatedRouteSnapshot, { url } as RouterStateSnapshot)) as Promise<boolean | UrlTree>;
}

const toString = (tree: boolean | UrlTree) => (typeof tree === 'boolean' ? tree : TestBed.inject(Router).serializeUrl(tree));

describe('route guards', () => {
  it('authGuard lets logged-in users through', async () => {
    expect(await run(authGuard, makeUser())).toBe(true);
  });

  it('authGuard sends visitors to /login and remembers where they were going', async () => {
    expect(toString(await run(authGuard, null, '/submit'))).toBe('/login?returnUrl=%2Fsubmit');
  });

  it('guestGuard keeps logged-in users off the login page', async () => {
    expect(toString(await run(guestGuard, makeUser()))).toBe('/feed');
    TestBed.resetTestingModule();
    expect(await run(guestGuard, null)).toBe(true);
  });

  it('adminGuard admits admins', async () => {
    expect(await run(adminGuard, makeUser({ is_admin: true }), '/admin')).toBe(true);
  });

  it('adminGuard turns regular users and visitors away to the home page', async () => {
    expect(toString(await run(adminGuard, makeUser(), '/admin'))).toBe('/');
    TestBed.resetTestingModule();
    expect(toString(await run(adminGuard, null, '/admin'))).toBe('/');
  });
});
