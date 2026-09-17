import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Game } from '../models/game';

@Injectable({
  providedIn: 'root',
})
export class GamesService {
  private apiUrl = 'http://localhost:8080/api';

  constructor(private http: HttpClient) {}

  getGames() {
    return this.http.get<Game[]>(`${this.apiUrl}/games`);
  }
}
