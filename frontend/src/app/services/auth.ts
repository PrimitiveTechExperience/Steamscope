import { Injectable, PLATFORM_ID, computed, inject, signal } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { HttpClient } from '@angular/common/http';
import { Observable, catchError, firstValueFrom, map, of, tap } from 'rxjs';

import { API_URL } from '../api';
import { User } from '../models/user';

interface UserResponse {
  user: User | null;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  private http = inject(HttpClient);
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));
  private initialCheck: Promise<User | null> | null = null;

  /** undefined until the first /auth/me check finishes; null when logged out. */
  readonly user = signal<User | null | undefined>(undefined);
  readonly isLoggedIn = computed(() => !!this.user());

  /**
   * Resolves once we know whether the visitor is logged in. Only the first
   * call hits the network; after that it reflects the live `user` signal, so
   * route guards see logins and logouts that happen mid-session. (Caching the
   * first answer forever made guards think you were still logged out after
   * logging in, and still logged in after logging out.)
   * Only meaningful in the browser - the SSR server never has the cookie.
   */
  ready(): Promise<User | null> {
    if (!this.isBrowser) {
      return Promise.resolve(null);
    }
    const current = this.user();
    if (current !== undefined) {
      return Promise.resolve(current);
    }
    this.initialCheck ??= this.refresh();
    return this.initialCheck;
  }

  refresh(): Promise<User | null> {
    return firstValueFrom(
      this.http.get<UserResponse>(`${API_URL}/auth/me`).pipe(
        map((res) => res.user),
        catchError(() => of(null)),
        tap((user) => this.user.set(user))
      )
    );
  }

  login(login: string, password: string): Observable<User> {
    return this.http
      .post<{ user: User }>(`${API_URL}/auth/login`, { login, password })
      .pipe(map((res) => res.user), tap((user) => this.user.set(user)));
  }

  register(username: string, email: string, password: string): Observable<User> {
    return this.http
      .post<{ user: User }>(`${API_URL}/auth/register`, { username, email, password })
      .pipe(map((res) => res.user), tap((user) => this.user.set(user)));
  }

  logout(): Observable<void> {
    return this.http.post<void>(`${API_URL}/auth/logout`, {}).pipe(tap(() => this.user.set(null)));
  }

  /** Full-page navigations - the OpenID flow redirects through Steam. */
  steamLoginUrl = `${API_URL}/auth/steam/login`;
  steamLinkUrl = `${API_URL}/auth/steam/link`;
}
