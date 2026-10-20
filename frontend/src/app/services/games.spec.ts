import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';

import { GamesService } from './games';
import { API_URL } from '../api';

describe('GamesService', () => {
  let service: GamesService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    service = TestBed.inject(GamesService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('sends search filters as query parameters, joining lists with commas', () => {
    service.getGames({ search: 'half life', genres: ['Action', 'RPG'], minPrice: 0, maxPrice: 20, limit: 5 }).subscribe();
    const req = http.expectOne((r) => r.url === `${API_URL}/games`);
    expect(req.request.params.get('search')).toBe('half life');
    expect(req.request.params.get('genres')).toBe('Action,RPG');
    expect(req.request.params.get('minPrice')).toBe('0'); // zero is a real filter, not "unset"
    expect(req.request.params.get('maxPrice')).toBe('20');
    expect(req.request.params.get('limit')).toBe('5');
    expect(req.request.params.has('tags')).toBe(false);
    req.flush({ games: [], total: 0, limit: 5, offset: 0 });
  });

  it('sends the minimum discount, and leaves it out when it is 0 or unset', () => {
    service.getGames({ minDiscount: 50 }).subscribe();
    const req = http.expectOne((r) => r.url === `${API_URL}/games`);
    expect(req.request.params.get('minDiscount')).toBe('50');
    req.flush({ games: [], total: 0, limit: 20, offset: 0 });

    service.getGames({ minDiscount: 0 }).subscribe();
    const none = http.expectOne((r) => r.url === `${API_URL}/games`);
    expect(none.request.params.has('minDiscount')).toBe(false);
    none.flush({ games: [], total: 0, limit: 20, offset: 0 });
  });

  it('asks for bundles only when told to', () => {
    service.getGames({ search: 'portal', includeBundles: true }).subscribe();
    const on = http.expectOne((r) => r.url === `${API_URL}/games`);
    expect(on.request.params.get('includeBundles')).toBe('true');
    expect(on.request.params.get('search')).toBe('portal');
    on.flush({ games: [], total: 0, limit: 20, offset: 0 });

    for (const opts of [{ includeBundles: false }, {}]) {
      service.getGames(opts).subscribe();
      const off = http.expectOne((r) => r.url === `${API_URL}/games`);
      expect(off.request.params.has('includeBundles')).toBe(false);
      off.flush({ games: [], total: 0, limit: 20, offset: 0 });
    }
  });

  it('returns the bundles the server sends back', () => {
    let got: unknown;
    service.getGames({ includeBundles: true }).subscribe((r) => (got = r));
    http.expectOne((r) => r.url === `${API_URL}/games`).flush({ games: [], total: 0, limit: 20, offset: 0, bundles: [{ bundle_id: 1, name: 'Pack' }] });
    expect((got as { bundles: unknown[] }).bundles).toHaveLength(1);
  });

  it('searches bundles, and leaves the parameter out when there is nothing to search for', () => {
    service.getBundles(undefined, '  portal  ').subscribe();
    const searched = http.expectOne((r) => r.url === `${API_URL}/bundles`);
    expect(searched.request.params.get('search')).toBe('portal');
    expect(searched.request.params.has('app_id')).toBe(false);
    searched.flush([]);

    for (const blank of [undefined, '', '   ']) {
      service.getBundles(undefined, blank).subscribe();
      const all = http.expectOne((r) => r.url === `${API_URL}/bundles`);
      expect(all.request.params.has('search')).toBe(false);
      all.flush([]);
    }

    service.getBundles(730).subscribe();
    const forGame = http.expectOne((r) => r.url === `${API_URL}/bundles`);
    expect(forGame.request.params.get('app_id')).toBe('730');
    forGame.flush([]);
  });

  it('omits empty filters entirely', () => {
    service.getGames({ search: '', genres: [] }).subscribe();
    const req = http.expectOne((r) => r.url === `${API_URL}/games`);
    expect(req.request.params.keys()).toEqual([]);
    req.flush({ games: [], total: 0, limit: 20, offset: 0 });
  });

  it('fetches one game and its price history', () => {
    service.getGame(730).subscribe();
    http.expectOne(`${API_URL}/games/730`).flush({});
    service.getPriceHistory(730).subscribe();
    http.expectOne(`${API_URL}/games/730/price-history`).flush([]);
  });

  it('fetches the price forecast and the buy-or-wait advice', () => {
    service.getPrediction(730).subscribe();
    http.expectOne(`${API_URL}/games/730/prediction`).flush({});
    service.getAdvice(730).subscribe();
    http.expectOne(`${API_URL}/games/730/advice`).flush({});
  });

  it('lists all bundles, or only those containing a game', () => {
    service.getBundles().subscribe();
    const all = http.expectOne((r) => r.url === `${API_URL}/bundles`);
    expect(all.request.params.has('app_id')).toBe(false);
    all.flush([]);

    service.getBundles(730).subscribe();
    const filtered = http.expectOne((r) => r.url === `${API_URL}/bundles`);
    expect(filtered.request.params.get('app_id')).toBe('730');
    filtered.flush([]);
  });

  it('fetches one bundle', () => {
    service.getBundle(5001).subscribe();
    http.expectOne(`${API_URL}/bundles/5001`).flush({});
  });
});
