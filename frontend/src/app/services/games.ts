import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';

import { Game, GamesResponse, PricePoint } from '../models/game';

@Injectable({
  providedIn: 'root',
})
export class GamesService {
  private http = inject(HttpClient);
  private apiUrl = 'http://localhost:8080/api';

  getGames(search?: string): Observable<GamesResponse> {
    let params = new HttpParams();
    if (search) {
      params = params.set('search', search);
    }
    return this.http.get<GamesResponse>(`${this.apiUrl}/games`, { params });
  }

  getGame(id: number): Observable<Game> {
    return this.http.get<Game>(`${this.apiUrl}/games/${id}`);
  }

  getPriceHistory(id: number): Observable<PricePoint[]> {
    return this.http.get<PricePoint[]>(`${this.apiUrl}/games/${id}/price-history`);
  }
}
