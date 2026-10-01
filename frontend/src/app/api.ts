import { HttpErrorResponse } from '@angular/common/http';

export const API_URL = 'http://localhost:8080/api';

/** Pulls the `{"error": "..."}` message out of an API error response. */
export function apiErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof HttpErrorResponse) {
    if (err.status === 0) return "Can't reach the server right now.";
    const message = err.error?.error;
    // Server messages are lower-case sentences ("you can submit up to 5...").
    if (typeof message === 'string' && message) return message.charAt(0).toUpperCase() + message.slice(1);
  }
  return fallback;
}
