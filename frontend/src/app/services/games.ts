import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';

import { FilterOptions, Game, GamesResponse, PricePoint } from '../models/game';

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
}

@Injectable({
  providedIn: 'root',
})
export class GamesService {
  private http = inject(HttpClient);
  private apiUrl = 'http://localhost:8080/api';

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
    return this.http.get<GamesResponse>(`${this.apiUrl}/games`, { params });
  }

  getGame(id: number): Observable<Game> {
    return this.http.get<Game>(`${this.apiUrl}/games/${id}`);
  }

  getPriceHistory(id: number): Observable<PricePoint[]> {
    return this.http.get<PricePoint[]>(`${this.apiUrl}/games/${id}/price-history`);
  }

  getFilterOptions(): Observable<FilterOptions> {
    return this.http.get<FilterOptions>(`${this.apiUrl}/filters`);
  }
}
