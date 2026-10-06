import { Component, input, output } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { of } from 'rxjs';

import { GameDetailComponent } from './game-detail';
import { PriceChartComponent } from '../../components/price-chart/price-chart';
import { PricePredictionComponent } from '../../components/price-prediction/price-prediction';
import { GamesService } from '../../services/games';
import { AuthService } from '../../services/auth';
import { AccountService } from '../../services/account';
import { Game } from '../../models/game';
import { Bundle } from '../../models/bundle';
import { asListedBundle, fakeAuth, makeBundle, makeGame, stubIntersectionObserver, textOf } from '../../../testing/factories';

@Component({ selector: 'app-price-chart', template: '<p class="chart-stub"></p>' })
class StubPriceChart {
  points = input<unknown[]>([]);
  loading = input(false);
}

@Component({ selector: 'app-price-prediction', template: '<p class="prediction-stub">{{ appId() }}</p>' })
class StubPrediction {
  appId = input<number>();
  refreshKey = input<unknown>();
  recordLowChange = output<boolean>();
}

async function render(game: Game, bundles: Bundle[] = []) {
  stubIntersectionObserver();
  const service = {
    getGame: vi.fn().mockReturnValue(of(game)),
    getPriceHistory: vi.fn().mockReturnValue(of([])),
    getGames: vi.fn().mockReturnValue(of({ games: [], total: 0, limit: 10, offset: 0 })),
    getBundles: vi.fn().mockReturnValue(of(bundles)),
  };
  TestBed.configureTestingModule({
    providers: [
      provideRouter([{ path: 'games/:app_id', component: GameDetailComponent }]),
      { provide: GamesService, useValue: service },
      { provide: AuthService, useValue: fakeAuth(null) },
      { provide: AccountService, useValue: {} },
    ],
  });
  TestBed.overrideComponent(GameDetailComponent, {
    remove: { imports: [PriceChartComponent, PricePredictionComponent] },
    add: { imports: [StubPriceChart, StubPrediction] },
  });
  const harness = await RouterTestingHarness.create();
  await harness.navigateByUrl(`/games/${game.app_id}`, GameDetailComponent);
  harness.detectChanges();
  return { harness, el: harness.routeNativeElement as HTMLElement, service };
}

const STEAM_HTML =
  '<h2>Gameplay</h2><p>Defuse the <strong>bomb</strong> with friends.</p>' +
  '<ul><li>Co-op</li><li>PvP</li></ul><a href="https://example.com" target="_blank" rel="noopener noreferrer nofollow">Site</a>' +
  '<img src="https://shared.akamai.steamstatic.com/a.jpg" alt="shot" />';

describe('GameDetailComponent', () => {
  it('renders the sanitized HTML description as real elements, not text', async () => {
    const { el } = await render(makeGame({ description_html: STEAM_HTML }));
    const box = el.querySelector('.steam-description')!;

    expect(box.querySelector('h2')?.textContent).toBe('Gameplay');
    expect(box.querySelector('strong')?.textContent).toBe('bomb');
    expect(Array.from(box.querySelectorAll('li')).map((l) => l.textContent)).toEqual(['Co-op', 'PvP']);
    expect(box.querySelector('a')?.getAttribute('href')).toBe('https://example.com');
    expect(box.querySelector('img')?.getAttribute('alt')).toBe('shot');
    // The tags must not appear as visible text.
    expect(textOf(box)).not.toMatch(/<\/?(h2|p|strong|ul|li)/);
  });

  it('shows plain text when only a plain description exists', async () => {
    const { el } = await render(makeGame({ description_html: undefined, description: 'Just plain words.' }));
    expect(el.querySelector('.steam-description')).toBeNull();
    expect(textOf(el.querySelector('p.whitespace-pre-line'))).toBe('Just plain words.');
  });

  it('strips script and event handlers from a hostile description (Angular sanitizes innerHTML)', async () => {
    const { el } = await render(makeGame({ description_html: '<p onclick="alert(1)">hi</p><script>alert(2)</script><img src=x onerror="alert(3)">' }));
    const html = el.querySelector('.steam-description')!.innerHTML;
    expect(html).not.toContain('onclick');
    expect(html).not.toContain('<script');
    expect(html).not.toContain('onerror');
    expect(html).toContain('hi');
  });

  it('shows the buy-now-or-wait panel for the game', async () => {
    const { el } = await render(makeGame({ app_id: 730 }));
    expect(textOf(el.querySelector('app-price-prediction .prediction-stub'))).toBe('730');
  });

  it('shows a "Record low" badge by the price once the panel reports one', async () => {
    const { harness, el } = await render(makeGame());
    expect(el.querySelector('.record-low-badge')).toBeNull();

    const panel = harness.fixture.debugElement.query((d) => d.name === 'app-price-prediction');
    panel.injector.get(StubPrediction).recordLowChange.emit(true);
    harness.detectChanges();
    expect(textOf(el.querySelector('.record-low-badge'))).toBe('Record low');

    panel.injector.get(StubPrediction).recordLowChange.emit(false);
    harness.detectChanges();
    expect(el.querySelector('.record-low-badge')).toBeNull();
  });

  it('shows the name, price, discount, developer and release date', async () => {
    const { el, service } = await render(makeGame());
    expect(service.getGame).toHaveBeenCalledWith(730);
    const text = textOf(el);
    expect(text).toContain('Counter-Strike 2');
    expect(text).toContain('$9.99');
    expect(text).toContain('$19.99');
    expect(text).toContain('Released');
    expect(text).toContain('Sep 27, 2023');
  });

  it('shows the release date as stored, whatever timezone the viewer is in', async () => {
    // Midnight UTC would read "Sep 1" in any timezone west of UTC if formatted locally.
    const { el } = await render(makeGame({ release_date: '2026-09-02T00:00:00Z' }));
    expect(textOf(el)).toContain('Sep 2, 2026');
  });

  it('hides the release date when it is unknown (year 1)', async () => {
    const { el } = await render(makeGame({ release_date: '0001-01-01T00:00:00Z' }));
    expect(textOf(el)).not.toContain('Released');
    expect(textOf(el)).not.toContain('Jan 1, 1');
  });

  it('lists the bundles that include this game, and queries by app id', async () => {
    const bundles = [asListedBundle(makeBundle()), asListedBundle(makeBundle({ bundle_id: 5002, name: 'Second Pack' }))];
    const { el, service } = await render(makeGame(), bundles);
    expect(service.getBundles).toHaveBeenCalledWith(730);
    expect(textOf(el)).toContain('Bundles including this game');
    const links = Array.from(el.querySelectorAll('app-bundle-card a')).map((a) => a.getAttribute('href'));
    expect(links).toEqual(['/bundles/5001', '/bundles/5002']);
  });

  it('omits the bundles section when no bundle includes the game', async () => {
    const { el } = await render(makeGame(), []);
    expect(textOf(el)).not.toContain('Bundles including this game');
  });

  it('switches to the tags tab and shows genres and tags as links to search', async () => {
    const { harness, el } = await render(makeGame({ genres: ['Action', 'RPG'], tags: ['FPS'] }));
    const tab = Array.from(el.querySelectorAll('button')).find((b) => textOf(b).startsWith('Tags'))!;
    tab.click();
    harness.detectChanges();
    const links = Array.from(el.querySelectorAll('a')).filter((a) => a.getAttribute('href')?.startsWith('/search'));
    expect(links.map((a) => textOf(a))).toEqual(['Action', 'RPG', 'FPS']);
    expect(links[0].getAttribute('href')).toContain('genres=Action');
  });
});
