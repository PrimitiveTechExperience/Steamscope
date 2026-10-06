import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';

import { FilterOptions, Game, GamesResponse, PricePoint } from '../models/game';
import { Bundle, BundleDetail } from '../models/bundle';
import { Advice, Forecast } from '../models/prediction';
import { API_URL } from '../api';

export interface GamesQueryOptions {
  search?: string;
  developer?: string;
  publisher?: string;
  genres?: string[];
  tags?: string[];
  languages?: string[];
  developers?: string[];
  publishers?: string[];
  minPrice?: number;
  maxPrice?: number;
  /** Only games at least this many percent off (1-100). */
  minDiscount?: number;
  limit?: number;
}

@Injectable({
  providedIn: 'root',
})
export class GamesService {
  private http = inject(HttpClient);
  private apiUrl = API_URL;

  getGames(opts?: GamesQueryOptions): Observable<GamesResponse> {
    let params = new HttpParams();
    if (opts?.search) params = params.set('search', opts.search);
    if (opts?.developer) params = params.set('developer', opts.developer);
    if (opts?.publisher) params = params.set('publisher', opts.publisher);
    if (opts?.genres?.length) params = params.set('genres', opts.genres.join(','));
    if (opts?.tags?.length) params = params.set('tags', opts.tags.join(','));
    if (opts?.languages?.length) params = params.set('languages', opts.languages.join(','));
    if (opts?.developers?.length) params = params.set('developers', opts.developers.join(','));
    if (opts?.publishers?.length) params = params.set('publishers', opts.publishers.join(','));
    if (opts?.minPrice != null) params = params.set('minPrice', opts.minPrice);
    if (opts?.maxPrice != null) params = params.set('maxPrice', opts.maxPrice);
    if (opts?.minDiscount != null && opts.minDiscount > 0) params = params.set('minDiscount', opts.minDiscount);
    if (opts?.limit != null) params = params.set('limit', opts.limit);
    return this.http.get<GamesResponse>(`${this.apiUrl}/games`, { params });
  }

  getGame(id: number): Observable<Game> {
    return this.http.get<Game>(`${this.apiUrl}/games/${id}`);
  }

  getPriceHistory(id: number): Observable<PricePoint[]> {
    return this.http.get<PricePoint[]>(`${this.apiUrl}/games/${id}/price-history`);
  }

  getBundles(appId?: number): Observable<Bundle[]> {
    let params = new HttpParams();
    if (appId != null) params = params.set('app_id', appId);
    return this.http.get<Bundle[]>(`${this.apiUrl}/bundles`, { params });
  }

  getBundle(id: number): Observable<BundleDetail> {
    return this.http.get<BundleDetail>(`${this.apiUrl}/bundles/${id}`);
  }

  /** Price forecast for up to two years ahead. */
  getPrediction(id: number): Observable<Forecast> {
    return this.http.get<Forecast>(`${this.apiUrl}/games/${id}/prediction`);
  }

  /** Buy-now-or-wait advice; personalised when the user is watching the game. */
  getAdvice(id: number): Observable<Advice> {
    return this.http.get<Advice>(`${this.apiUrl}/games/${id}/advice`);
  }

  /** Price forecast for a bundle; same shape as a game's. */
  getBundlePrediction(id: number): Observable<Forecast> {
    return this.http.get<Forecast>(`${this.apiUrl}/bundles/${id}/prediction`);
  }

  /** Buy-now-or-wait advice for a bundle (always the general call). */
  getBundleAdvice(id: number): Observable<Advice> {
    return this.http.get<Advice>(`${this.apiUrl}/bundles/${id}/advice`);
  }

  getFilterOptions(): Observable<FilterOptions> {
    return this.http.get<FilterOptions>(`${this.apiUrl}/filters`);
  }
}
