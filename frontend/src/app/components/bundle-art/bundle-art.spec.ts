import { TestBed } from '@angular/core/testing';

import { BundleArtComponent } from './bundle-art';
import { asListedBundle, makeBundle, textOf } from '../../../testing/factories';

function render(over = {}) {
  TestBed.configureTestingModule({ imports: [BundleArtComponent] });
  const fixture = TestBed.createComponent(BundleArtComponent);
  fixture.componentRef.setInput('bundle', asListedBundle(makeBundle(over)));
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement };
}

const GAMES = [1, 2, 3, 4, 5].map((id) => ({ app_id: id, name: `Game ${id}`, tracked: true, track_status: '' as const, price: 10, regular_price: 20 }));

describe('BundleArtComponent', () => {
  it("shows Steam's header image when the bundle has one", () => {
    const { el } = render();
    const imgs = el.querySelectorAll('img');
    expect(imgs).toHaveLength(1);
    expect(imgs[0].getAttribute('src')).toBe('https://cdn.example/bundle/5001.jpg');
  });

  it('builds a collage from the bundle games when there is no header image', () => {
    const { el } = render({ header_image: '', games: GAMES });
    const srcs = Array.from(el.querySelectorAll('img')).map((i) => i.getAttribute('src'));
    expect(srcs).toHaveLength(4); // capped at four tiles
    expect(srcs[0]).toContain('/steam/apps/1/header.jpg');
    expect(srcs[3]).toContain('/steam/apps/4/header.jpg');
    expect(el.querySelector('[role="img"]')?.getAttribute('aria-label')).toBe('Starter Pack');
  });

  it('uses a single full-width tile for a one-game bundle', () => {
    const { el } = render({ header_image: '', games: GAMES.slice(0, 1) });
    expect(el.querySelectorAll('img')).toHaveLength(1);
    expect(el.querySelector('.grid-cols-1')).not.toBeNull();
  });

  it('falls back to the collage when the header image fails to load', () => {
    const { fixture, el } = render({ games: GAMES.slice(0, 2) });
    el.querySelector('img')!.dispatchEvent(new Event('error'));
    fixture.detectChanges();
    const srcs = Array.from(el.querySelectorAll('img')).map((i) => i.getAttribute('src'));
    expect(srcs).toHaveLength(2);
    expect(srcs.every((s) => s!.includes('/steam/apps/'))).toBe(true);
  });

  it('drops a collage tile whose image fails to load', () => {
    const { fixture, el } = render({ header_image: '', games: GAMES.slice(0, 3) });
    el.querySelectorAll('img')[1].dispatchEvent(new Event('error'));
    fixture.detectChanges();
    expect(el.querySelectorAll('img')).toHaveLength(2);
  });

  it('shows a placeholder when there is nothing to draw', () => {
    const { el } = render({ header_image: '', games: [] });
    expect(el.querySelector('img')).toBeNull();
    expect(textOf(el)).toBe('Bundle');
  });
});
