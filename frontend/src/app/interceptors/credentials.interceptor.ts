import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { Router } from '@angular/router';
import { catchError, throwError } from 'rxjs';

import { API_URL } from '../api';
import { AuthService } from '../services/auth';

// The API lives on a different origin (8080 vs 4200), so the browser only
// sends/stores the session cookie when requests opt in with credentials.
export const credentialsInterceptor: HttpInterceptorFn = (req, next) => {
  if (!req.url.startsWith(API_URL)) {
    return next(req);
  }
  const auth = inject(AuthService);
  const router = inject(Router);

  return next(req.clone({ withCredentials: true })).pipe(
    catchError((err: unknown) => {
      // A 401 from a logged-in-only endpoint means the session is gone
      // (expired, or logged out in another tab). /auth/* 401s are just bad
      // credentials and are handled by the login form itself.
      const isAuthEndpoint = req.url.startsWith(`${API_URL}/auth/`);
      if (err instanceof HttpErrorResponse && err.status === 401 && !isAuthEndpoint && auth.user()) {
        auth.user.set(null);
        router.navigate(['/login'], { queryParams: { returnUrl: router.url } });
      }
      return throwError(() => err);
    })
  );
};
