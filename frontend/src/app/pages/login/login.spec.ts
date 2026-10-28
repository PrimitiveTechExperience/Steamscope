import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { throwError } from 'rxjs';

import { LoginComponent } from './login';
import { AuthService } from '../../services/auth';
import { textOf } from '../../../testing/factories';

async function render(query: Record<string, string> = {}, login = vi.fn()) {
  TestBed.configureTestingModule({
    imports: [LoginComponent],
    providers: [
      provideRouter([]),
      { provide: AuthService, useValue: { login, steamLoginUrl: '/steam' } },
      { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap(query) } } },
    ],
  });
  const fixture = TestBed.createComponent(LoginComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  return { fixture, el: fixture.nativeElement as HTMLElement };
}

describe('LoginComponent', () => {
  it('links to the forgot-password page', async () => {
    const { el } = await render();
    expect(textOf(el.querySelector('a[href="/forgot-password"]'))).toBe('Forgot your password?');
  });

  it('confirms a password reset', async () => {
    const { el } = await render({ reset: 'done' });
    expect(textOf(el.querySelector('.notice-message'))).toBe('Your password has been changed. Log in with the new one.');
  });

  it('shows no notice otherwise', async () => {
    expect((await render()).el.querySelector('.notice-message')).toBeNull();
  });

  it('shows the lockout message from the server', async () => {
    const login = vi.fn().mockReturnValue(
      throwError(() => new HttpErrorResponse({
        status: 429,
        error: { error: 'too many failed sign-in attempts for this account. Try again in 30 minutes, or reset your password' },
      }))
    );
    const { el, fixture } = await render({}, login);
    for (const [name, value] of [['login', 'me'], ['password', 'wrong-password']]) {
      const input = el.querySelector<HTMLInputElement>(`input[name="${name}"]`)!;
      input.value = value;
      input.dispatchEvent(new Event('input'));
    }
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
    expect(textOf(el)).toContain('Too many failed sign-in attempts for this account. Try again in 30 minutes, or reset your password');
  });
});
