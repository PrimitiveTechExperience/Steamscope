import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { Location } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { of, throwError } from 'rxjs';

import { ResetPasswordComponent } from './reset-password';
import { AuthService } from '../../services/auth';
import { textOf } from '../../../testing/factories';

const TOKEN = 'x'.repeat(43);

async function render(token: string | null, confirmPasswordReset = vi.fn().mockReturnValue(of('changed'))) {
  const replaceState = vi.fn();
  TestBed.configureTestingModule({
    imports: [ResetPasswordComponent],
    providers: [
      provideRouter([]),
      { provide: AuthService, useValue: { confirmPasswordReset } },
      { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap(token === null ? {} : { token }) } } },
      { provide: Location, useValue: { replaceState } },
    ],
  });
  const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
  const fixture = TestBed.createComponent(ResetPasswordComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  const el = fixture.nativeElement as HTMLElement;
  const submit = async (password: string, confirm: string) => {
    for (const [name, value] of [['password', password], ['confirm', confirm]]) {
      const input = el.querySelector<HTMLInputElement>(`input[name="${name}"]`)!;
      input.value = value;
      input.dispatchEvent(new Event('input'));
    }
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
    await fixture.whenStable();
  };
  return { el, submit, confirmPasswordReset, replaceState, navigate };
}

describe('ResetPasswordComponent', () => {
  it('takes the token out of the address bar straight away', async () => {
    const { replaceState } = await render(TOKEN);
    expect(replaceState).toHaveBeenCalledWith('/reset-password');
  });

  it('sets the new password and sends the user to log in', async () => {
    const { submit, confirmPasswordReset, navigate } = await render(TOKEN);
    await submit('a-new-password', 'a-new-password');
    expect(confirmPasswordReset).toHaveBeenCalledWith(TOKEN, 'a-new-password');
    expect(navigate).toHaveBeenCalledWith(['/login'], { queryParams: { reset: 'done' } });
  });

  it('checks the password before sending it', async () => {
    const { el, submit, confirmPasswordReset } = await render(TOKEN);
    await submit('short', 'short');
    expect(textOf(el.querySelector('.error-message'))).toBe('Your new password must be 8-128 characters.');
    await submit('a-new-password', 'a-different-one');
    expect(textOf(el.querySelector('.error-message'))).toBe("The two passwords don't match.");
    expect(confirmPasswordReset).not.toHaveBeenCalled();
  });

  it('offers a new link when this one has expired', async () => {
    const fail = vi.fn().mockReturnValue(
      throwError(() => new HttpErrorResponse({ status: 400, error: { error: 'this reset link is invalid or has expired. Ask for a new one' } }))
    );
    const { el, submit } = await render(TOKEN, fail);
    await submit('a-new-password', 'a-new-password');
    expect(textOf(el.querySelector('.error-message'))).toContain('This reset link is invalid or has expired');
    expect(el.querySelector('.error-message a[href="/forgot-password"]')).not.toBeNull();
  });

  it('explains a link with no token, without a form', async () => {
    const { el, replaceState } = await render(null);
    expect(el.querySelector('form')).toBeNull();
    expect(textOf(el.querySelector('.error-message'))).toContain('This link is incomplete');
    expect(replaceState).not.toHaveBeenCalled();
  });
});
