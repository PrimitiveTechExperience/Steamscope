import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { of, throwError } from 'rxjs';

import { ForgotPasswordComponent } from './forgot-password';
import { AuthService } from '../../services/auth';
import { textOf } from '../../../testing/factories';

const SENT = 'If an account uses that email, a link to reset its password is on its way. It works for one hour.';

async function render(requestPasswordReset = vi.fn().mockReturnValue(of(SENT))) {
  TestBed.configureTestingModule({
    imports: [ForgotPasswordComponent],
    providers: [provideRouter([]), { provide: AuthService, useValue: { requestPasswordReset } }],
  });
  const fixture = TestBed.createComponent(ForgotPasswordComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  const el = fixture.nativeElement as HTMLElement;
  const submit = async (email: string) => {
    const input = el.querySelector<HTMLInputElement>('input[name="email"]')!;
    input.value = email;
    input.dispatchEvent(new Event('input'));
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
    await fixture.whenStable();
  };
  return { fixture, el, submit, requestPasswordReset };
}

describe('ForgotPasswordComponent', () => {
  it("shows the server's answer, which is the same for any address", async () => {
    const { el, submit, requestPasswordReset } = await render();
    await submit('  me@example.com ');
    expect(requestPasswordReset).toHaveBeenCalledWith('me@example.com');
    expect(textOf(el.querySelector('.sent-message'))).toBe(SENT);
    expect(el.querySelector('form')).toBeNull();
  });

  it('asks for an email before sending', async () => {
    const { el, submit, requestPasswordReset } = await render();
    await submit('   ');
    expect(requestPasswordReset).not.toHaveBeenCalled();
    expect(textOf(el.querySelector('.error-message'))).toBe('Enter the email address of your account.');
  });

  it("says so when the server can't send email", async () => {
    const fail = vi.fn().mockReturnValue(
      throwError(() => new HttpErrorResponse({ status: 503, error: { error: "password reset by email isn't set up on this server. Ask an admin for help" } }))
    );
    const { el, submit } = await render(fail);
    await submit('me@example.com');
    expect(textOf(el.querySelector('.error-message'))).toContain("Password reset by email isn't set up");
    expect(el.querySelector('form')).not.toBeNull();
  });

  it('can be tried again after sending', async () => {
    const { el, fixture, submit } = await render();
    await submit('me@example.com');
    (Array.from(el.querySelectorAll('button')).find((b) => textOf(b) === 'try again') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(el.querySelector('form')).not.toBeNull();
  });

  it('links back to the login page', async () => {
    const { el } = await render();
    expect(el.querySelector('a[href="/login"]')).not.toBeNull();
  });
});
