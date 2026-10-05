import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { HttpErrorResponse } from '@angular/common/http';

import { SubmitComponent } from './submit';
import { AccountService } from '../../services/account';
import { Submission } from '../../models/user';
import { textOf } from '../../../testing/factories';

const SUBMISSIONS: Submission[] = [
  { kind: 'app', id: 730, name: 'Counter-Strike 2', status: 'tracked', created_at: '2026-10-01T12:00:00Z' },
  { kind: 'bundle', id: 5001, name: 'Starter Pack', status: 'tracked', created_at: '2026-10-01T12:00:00Z' },
  { kind: 'app', id: 111, name: null, status: 'awaiting_approval', created_at: '2026-10-01T12:00:00Z' },
  { kind: 'app', id: 222, name: null, status: 'rejected', created_at: '2026-10-01T12:00:00Z' },
  { kind: 'bundle', id: 333, name: null, status: 'failed', created_at: '2026-10-01T12:00:00Z' },
];

function render(account: Record<string, unknown> = {}) {
  const service = {
    getSubmissions: vi.fn().mockReturnValue(of(SUBMISSIONS)),
    submitGame: vi.fn().mockReturnValue(of({ kind: 'app', id: 730, status: 'awaiting_approval' })),
    ...account,
  };
  TestBed.configureTestingModule({
    imports: [SubmitComponent],
    providers: [provideRouter([]), { provide: AccountService, useValue: service }],
  });
  const fixture = TestBed.createComponent(SubmitComponent);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;

  async function submit(url: string) {
    await fixture.whenStable(); // ngModel registers asynchronously
    const input = el.querySelector('input[name="url"]') as HTMLInputElement;
    input.value = url;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    await fixture.whenStable();
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
  }
  const message = () => textOf(el.querySelector('p.border-2'));
  const rows = () => Array.from(el.querySelectorAll('ul li'));
  return { fixture, el, service, submit, message, rows };
}

describe('SubmitComponent', () => {
  it('explains that submissions need approval and mentions bundles', () => {
    const text = textOf(render().el);
    expect(text).toContain('Submit a game or bundle');
    expect(text).toContain('An admin approves each submission');
    expect(text).toContain('Up to 5 submissions an hour');
  });

  it('rejects links that are not Steam store game or bundle pages without calling the API', async () => {
    for (const bad of ['', 'hello', 'https://evil.example/app/730', 'https://store.steampowered.com/', 'https://store.steampowered.com/app/abc']) {
      TestBed.resetTestingModule();
      const { service, submit, message } = render();
      await submit(bad);
      expect(message(), bad).toContain('Paste a Steam game or bundle link');
      expect(service.submitGame, bad).not.toHaveBeenCalled();
    }
  });

  it.each([
    'https://store.steampowered.com/app/730/Counter-Strike_2/',
    'http://store.steampowered.com/app/730',
    'https://store.steampowered.com/bundle/232/Valve_Complete_Pack/',
  ])('accepts %s', async (url) => {
    const { service, submit } = render();
    await submit(url);
    expect(service.submitGame).toHaveBeenCalledWith(url);
  });

  it('tells the user an admin will review a new submission, clears the box and reloads the list', async () => {
    const { el, service, submit, message } = render();
    await submit('https://store.steampowered.com/app/730');
    expect(message()).toContain('An admin will review your submission');
    expect((el.querySelector('input[name="url"]') as HTMLInputElement).value).toBe('');
    expect(service.getSubmissions).toHaveBeenCalledTimes(2);
  });

  it.each([
    ['tracked', 'Good news - that is already being tracked.'],
    ['rejected', "That one was reviewed before and wasn't approved."],
    ['pending', "We're fetching that now."],
  ])('explains a %s result', async (status, expected) => {
    const { submit, message } = render({ submitGame: vi.fn().mockReturnValue(of({ kind: 'app', id: 1, status })) });
    await submit('https://store.steampowered.com/app/1');
    expect(message()).toContain(expected);
  });

  it('shows the server error, capitalized, such as the rate limit', async () => {
    const { submit, message } = render({
      submitGame: vi.fn().mockReturnValue(throwError(() => new HttpErrorResponse({ status: 429, error: { error: 'you can submit up to 5 games or bundles an hour' } }))),
    });
    await submit('https://store.steampowered.com/app/730');
    expect(message()).toBe('You can submit up to 5 games or bundles an hour');
  });

  it('lists submissions with a friendly label for each status', () => {
    const { rows } = render();
    const text = rows().map((r) => textOf(r));
    expect(text[0]).toContain('Counter-Strike 2');
    expect(text[0]).toContain('tracked');
    expect(text[2]).toContain('Awaiting approval');
    expect(text[3]).toContain('Not approved');
    expect(text[4]).toContain('failed');
  });

  it('links tracked games and bundles to their pages; unnamed ones show kind and id', () => {
    const { rows } = render();
    expect(rows()[0].querySelector('a')?.getAttribute('href')).toBe('/games/730');
    expect(rows()[1].querySelector('a')?.getAttribute('href')).toBe('/bundles/5001');
    expect(rows()[2].querySelector('a')).toBeNull();
    expect(textOf(rows()[2])).toContain('App 111');
    expect(textOf(rows()[4])).toContain('Bundle 333');
  });

  it('shows an empty state when nothing was submitted', () => {
    const { el } = render({ getSubmissions: vi.fn().mockReturnValue(of([])) });
    expect(textOf(el)).toContain("You haven't submitted anything yet.");
  });

  it('shows an empty list rather than a skeleton if loading fails', () => {
    const { el } = render({ getSubmissions: vi.fn().mockReturnValue(throwError(() => new HttpErrorResponse({ status: 500 }))) });
    expect(el.querySelector('.skeleton')).toBeNull();
    expect(textOf(el)).toContain("You haven't submitted anything yet.");
  });
});
