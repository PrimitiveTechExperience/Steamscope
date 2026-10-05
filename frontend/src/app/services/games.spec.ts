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
