import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';

import { AccountService } from './account';
import { API_URL } from '../api';

describe('AccountService wishlist', () => {
  let service: AccountService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    service = TestBed.inject(AccountService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('imports the wishlist with a POST', () => {
    let got: unknown;
    service.importWishlist().subscribe((r) => (got = r));
    const req = http.expectOne(`${API_URL}/me/wishlist/import`);
    expect(req.request.method).toBe('POST');
    req.flush({ wishlist_size: 2, watched: 2, already_watched: 0, requestable: [], awaiting_review: 0, unavailable: 0, state: 'complete' });
    expect(got).toMatchObject({ watched: 2 });
  });

  it('requests the chosen games by app id', () => {
    service.requestWishlistGames([11, 13]).subscribe();
    const req = http.expectOne(`${API_URL}/me/wishlist/request`);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ app_ids: [11, 13] }); // no empty pin list
    req.flush({ requested: 2, skipped: 0, status: 'awaiting_approval', state: 'waiting' });
  });

  it('sends which of the requested games to pin', () => {
    service.requestWishlistGames([11, 13], [13]).subscribe();
    const req = http.expectOne(`${API_URL}/me/wishlist/request`);
    expect(req.request.body).toEqual({ app_ids: [11, 13], pinned_app_ids: [13] });
    req.flush({ requested: 2, skipped: 0, status: 'awaiting_approval', state: 'waiting' });
  });

  it('reads the wishlist status with a GET', () => {
    let got: unknown;
    service.getWishlistStatus().subscribe((r) => (got = r));
    const req = http.expectOne(`${API_URL}/me/wishlist/status`);
    expect(req.request.method).toBe('GET');
    req.flush({ state: 'complete', wishlist_size: 3, remaining: 0, waiting: 0, unavailable: 0 });
    expect(got).toMatchObject({ state: 'complete' });
  });
});
