import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { HttpErrorResponse } from '@angular/common/http';

import { AdminGamesComponent } from './admin-games';
import { AdminService } from '../../../services/admin';
import { AdminItem } from '../../../models/admin';
import { textOf } from '../../../../testing/factories';

const ITEMS: AdminItem[] = [
  { kind: 'app', id: 111, name: null, status: 'awaiting_approval', submitted_by: 'alice', created_at: '2026-10-01T12:00:00Z' },
  { kind: 'bundle', id: 222, name: null, status: 'awaiting_approval', submitted_by: null, created_at: '2026-10-01T13:00:00Z' },
  { kind: 'app', id: 730, name: 'Counter-Strike 2', status: 'tracked', submitted_by: 'bob', created_at: '2026-09-01T00:00:00Z' },
  { kind: 'bundle', id: 5001, name: 'Starter Pack', status: 'tracked', submitted_by: null, created_at: '2026-09-02T00:00:00Z' },
  { kind: 'app', id: 999, name: null, status: 'failed', submitted_by: 'carol', created_at: '2026-09-03T00:00:00Z' },
  { kind: 'app', id: 998, name: null, status: 'rejected', submitted_by: 'carol', created_at: '2026-09-04T00:00:00Z' },
];

function render(admin: Record<string, unknown> = {}) {
  const service = {
    getItems: vi.fn().mockReturnValue(of(ITEMS)),
    approveItem: vi.fn().mockReturnValue(of(undefined)),
    rejectItem: vi.fn().mockReturnValue(of(undefined)),
    deleteItem: vi.fn().mockReturnValue(of(undefined)),
    ...admin,
  };
  TestBed.configureTestingModule({
    imports: [AdminGamesComponent],
    providers: [provideRouter([]), { provide: AdminService, useValue: service }],
  });
  const fixture = TestBed.createComponent(AdminGamesComponent);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const lists = () => Array.from(el.querySelectorAll('ul'));
  const awaiting = () => Array.from(lists()[0]?.querySelectorAll('li') ?? []);
  const others = () => Array.from(lists()[1]?.querySelectorAll('li') ?? []);
  const button = (r: Element, label: string) => Array.from(r.querySelectorAll('button')).find((b) => textOf(b) === label) as HTMLButtonElement | undefined;
  const filter = (label: string) => Array.from(el.querySelectorAll('button')).find((b) => textOf(b) === label) as HTMLButtonElement;
  return { fixture, el, service, awaiting, others, button, filter };
}

describe('AdminGamesComponent', () => {
  beforeEach(() => vi.stubGlobal('confirm', vi.fn().mockReturnValue(true)));
  afterEach(() => vi.unstubAllGlobals());

  it('separates submissions awaiting approval from everything else', () => {
    const { el, awaiting, others } = render();
    expect(awaiting()).toHaveLength(2);
    expect(others()).toHaveLength(4);
    expect(textOf(el)).toContain('Awaiting approval (2)');
    expect(textOf(el)).toContain('Games and bundles (4)');
  });

  it('shows who submitted each pending item and links to its Steam page', () => {
    const { awaiting } = render();
    const [game, bundle] = awaiting();
    expect(textOf(game)).toContain('App 111');
    expect(textOf(game)).toContain('by alice');
    expect(game.querySelector('a')?.getAttribute('href')).toBe('https://store.steampowered.com/app/111');
    expect(bundle.querySelector('a')?.getAttribute('href')).toBe('https://store.steampowered.com/bundle/222');
    expect(textOf(bundle)).toContain('by a deleted user');
    expect(game.querySelector('a')?.getAttribute('rel')).toContain('noopener');
  });

  it('approves and rejects, then reloads', () => {
    const { service, awaiting, button } = render();
    button(awaiting()[0], 'Approve')!.click();
    expect(service.approveItem).toHaveBeenCalledWith(ITEMS[0]);
    button(awaiting()[1], 'Reject')!.click();
    expect(service.rejectItem).toHaveBeenCalledWith(ITEMS[1]);
    expect(service.getItems).toHaveBeenCalledTimes(3);
  });

  it('links tracked items to their pages and shows kind, status and submitter', () => {
    const { others } = render();
    const game = others().find((r) => textOf(r).includes('Counter-Strike 2'))!;
    expect(game.querySelector('a')?.getAttribute('href')).toBe('/games/730');
    expect(textOf(game)).toContain('Game · tracked');
    expect(textOf(game)).toContain('submitted by bob');

    const bundle = others().find((r) => textOf(r).includes('Starter Pack'))!;
    expect(bundle.querySelector('a')?.getAttribute('href')).toBe('/bundles/5001');
    expect(textOf(bundle)).toContain('Bundle · tracked');

    const failed = others().find((r) => textOf(r).includes('App 999'))!;
    expect(failed.querySelector('a')).toBeNull();
  });

  it('filters by status', () => {
    const { fixture, others, filter } = render();
    filter('failed').click();
    fixture.detectChanges();
    expect(others().map((r) => textOf(r))).toEqual([expect.stringContaining('App 999')]);
    filter('rejected').click();
    fixture.detectChanges();
    expect(others()).toHaveLength(1);
    filter('all').click();
    fixture.detectChanges();
    expect(others()).toHaveLength(4);
  });

  it('searches by name, id and submitter', () => {
    const { fixture, el, others } = render();
    const input = el.querySelector('input[type="search"]') as HTMLInputElement;
    for (const [term, expected] of [['counter', 1], ['5001', 1], ['carol', 2], ['zzz', 0]] as const) {
      input.value = term;
      input.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      expect(others(), term).toHaveLength(expected);
    }
    expect(textOf(el)).toContain('Nothing matches.');
  });

  it('deletes only after confirmation, naming what is deleted', () => {
    const confirmSpy = vi.fn().mockReturnValue(false);
    vi.stubGlobal('confirm', confirmSpy);
    const { service, others, button } = render();
    const row = others().find((r) => textOf(r).includes('Counter-Strike 2'))!;

    button(row, 'Delete')!.click();
    expect(confirmSpy.mock.calls[0][0]).toContain('Counter-Strike 2');
    expect(service.deleteItem).not.toHaveBeenCalled();

    confirmSpy.mockReturnValue(true);
    button(row, 'Delete')!.click();
    expect(service.deleteItem).toHaveBeenCalledWith(ITEMS[2]);
  });

  it('shows an empty state when nothing needs review', () => {
    const { el } = render({ getItems: vi.fn().mockReturnValue(of([])) });
    expect(textOf(el)).toContain('Nothing to review.');
    expect(textOf(el)).toContain('Nothing matches.');
  });

  it('shows the server error when an action fails', () => {
    const { fixture, el, awaiting, button } = render({
      approveItem: vi.fn().mockReturnValue(throwError(() => new HttpErrorResponse({ status: 503, error: { error: 'too many submissions are being processed right now, try again shortly' } }))),
    });
    button(awaiting()[0], 'Approve')!.click();
    fixture.detectChanges();
    expect(textOf(el.querySelector('p.border-2'))).toBe('Too many submissions are being processed right now, try again shortly');
  });
});
