import { TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { HttpErrorResponse } from '@angular/common/http';

import { AdminBlacklistComponent } from './admin-blacklist';
import { AdminService } from '../../../services/admin';
import { BlacklistRule } from '../../../models/admin';
import { textOf } from '../../../../testing/factories';

const RULE: BlacklistRule = {
  rule_id: 7, field: 'developer', pattern: '^shady games', note: 'spam studio', created_by: 'root', created_at: '2026-10-01T12:00:00Z',
};

function render(admin: Record<string, unknown> = {}) {
  const service = {
    getBlacklist: vi.fn().mockReturnValue(of([RULE])),
    addBlacklistRule: vi.fn().mockReturnValue(of(RULE)),
    deleteBlacklistRule: vi.fn().mockReturnValue(of(undefined)),
    getBlacklistMatches: vi.fn().mockReturnValue(of([])),
    purgeBlacklistMatches: vi.fn().mockReturnValue(of({ removed: 2 })),
    ...admin,
  };
  TestBed.configureTestingModule({ imports: [AdminBlacklistComponent], providers: [{ provide: AdminService, useValue: service }] });
  const fixture = TestBed.createComponent(AdminBlacklistComponent);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const button = (label: string) => Array.from(el.querySelectorAll('button')).find((b) => textOf(b) === label) as HTMLButtonElement | undefined;
  return { fixture, el, service, button };
}

async function fill(fixture: ReturnType<typeof render>['fixture'], el: HTMLElement, values: { field?: string; pattern?: string; note?: string }) {
  await fixture.whenStable(); // ngModel registers its controls asynchronously
  const set = (selector: string, value: string, event: string) => {
    const control = el.querySelector(selector) as HTMLInputElement;
    control.value = value;
    control.dispatchEvent(new Event(event));
  };
  if (values.field) set('select', values.field, 'change');
  if (values.pattern !== undefined) set('input[name="pattern"]', values.pattern, 'input');
  if (values.note !== undefined) set('input[name="note"]', values.note, 'input');
  fixture.detectChanges();
  await fixture.whenStable();
}

describe('AdminBlacklistComponent', () => {
  beforeEach(() => vi.stubGlobal('confirm', vi.fn().mockReturnValue(true)));
  afterEach(() => vi.unstubAllGlobals());

  it('lists existing rules with field, pattern, note, author and date', () => {
    const { el } = render();
    const text = textOf(el.querySelector('ul')!);
    expect(text).toContain('Developer');
    expect(text).toContain('^shady games');
    expect(text).toContain('spam studio');
    expect(text).toContain('by root');
    expect(text).toContain('Oct 1, 2026');
    expect(textOf(el)).toContain('Rules (1)');
  });

  it('shows an empty state with no rules', () => {
    const { el } = render({ getBlacklist: vi.fn().mockReturnValue(of([])) });
    expect(textOf(el)).toContain('No rules yet.');
  });

  it('offers all four rule types and a placeholder that fits the chosen one', async () => {
    const { fixture, el } = render();
    expect(Array.from(el.querySelectorAll('option')).map((o) => textOf(o))).toEqual(['App ID', 'Game name', 'Developer', 'Publisher']);
    await fill(fixture, el, { field: 'app_id' });
    expect(el.querySelector('input[name="pattern"]')?.getAttribute('placeholder')).toBe('e.g. 730');
    await fill(fixture, el, { field: 'name' });
    expect(el.querySelector('input[name="pattern"]')?.getAttribute('placeholder')).toContain('regex');
  });

  it('adds a rule with the trimmed pattern, clears the form and reloads', async () => {
    const { fixture, el, service, button } = render();
    await fill(fixture, el, { field: 'publisher', pattern: '  ^bad pub  ', note: ' reason ' });
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();

    expect(service.addBlacklistRule).toHaveBeenCalledWith({ field: 'publisher', pattern: '^bad pub', note: 'reason' });
    expect(service.getBlacklist).toHaveBeenCalledTimes(2);
    expect(service.getBlacklistMatches).toHaveBeenCalledWith(7); // previews impact right away
    await fixture.whenStable();
    expect((el.querySelector('input[name="pattern"]') as HTMLInputElement).value).toBe('');
    expect(button('Add rule')).toBeDefined();
  });

  it('shows the server error for an invalid rule and keeps the form', async () => {
    const { fixture, el } = render({
      addBlacklistRule: vi.fn().mockReturnValue(throwError(() => new HttpErrorResponse({ status: 400, error: { error: 'invalid regular expression: missing closing )' } }))),
    });
    await fill(fixture, el, { pattern: '(' });
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
    expect(textOf(el.querySelector('p.border-2'))).toBe('Invalid regular expression: missing closing )');
    expect((el.querySelector('input[name="pattern"]') as HTMLInputElement).value).toBe('(');
  });

  it('previews which games on the site a rule would remove', () => {
    const { fixture, el, service, button } = render({
      getBlacklistMatches: vi.fn().mockReturnValue(of([{ app_id: 1, name: 'Shady Shooter' }, { app_id: 2, name: 'Shady Racer' }])),
    });
    button('Preview matches')!.click();
    fixture.detectChanges();
    expect(service.getBlacklistMatches).toHaveBeenCalledWith(7);
    const text = textOf(el.querySelector('ul')!);
    expect(text).toContain('2 game(s) on the site match');
    expect(text).toContain('Shady Shooter');
    expect(text).toContain('Shady Racer');
    expect(button('Remove these games')).toBeDefined();
  });

  it('says so when nothing matches, and offers no purge button', () => {
    const { fixture, el, button } = render();
    button('Preview matches')!.click();
    fixture.detectChanges();
    expect(textOf(el)).toContain('No games on the site match this rule.');
    expect(button('Remove these games')).toBeUndefined();
  });

  it('truncates a long match list to 30 names with a count of the rest', () => {
    const many = Array.from({ length: 35 }, (_, i) => ({ app_id: i, name: `Game ${i}` }));
    const { fixture, el, button } = render({ getBlacklistMatches: vi.fn().mockReturnValue(of(many)) });
    button('Preview matches')!.click();
    fixture.detectChanges();
    expect(textOf(el.querySelector('ul')!)).toContain('and 5 more');
    expect(el.querySelectorAll('ul ul li')).toHaveLength(31);
  });

  it('purges only after confirmation and reports how many were removed', () => {
    const confirmSpy = vi.fn().mockReturnValue(false);
    vi.stubGlobal('confirm', confirmSpy);
    const { fixture, el, service, button } = render({ getBlacklistMatches: vi.fn().mockReturnValue(of([{ app_id: 1, name: 'A' }, { app_id: 2, name: 'B' }])) });
    button('Preview matches')!.click();
    fixture.detectChanges();

    button('Remove these games')!.click();
    expect(service.purgeBlacklistMatches).not.toHaveBeenCalled();
    expect(confirmSpy.mock.calls[0][0]).toContain('2 game(s)');

    confirmSpy.mockReturnValue(true);
    button('Remove these games')!.click();
    fixture.detectChanges();
    expect(service.purgeBlacklistMatches).toHaveBeenCalledWith(7);
    expect(textOf(el)).toContain('Removed 2 game(s).');
    expect(textOf(el)).toContain('No games on the site match this rule.');
  });

  it('removes a rule after confirmation', () => {
    const { service, button } = render();
    button('Remove rule')!.click();
    expect(service.deleteBlacklistRule).toHaveBeenCalledWith(7);
    expect(service.getBlacklist).toHaveBeenCalledTimes(2);
  });

  it('does not remove a rule when the confirmation is declined', () => {
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false));
    const { service, button } = render();
    button('Remove rule')!.click();
    expect(service.deleteBlacklistRule).not.toHaveBeenCalled();
  });
});
