import { HttpErrorResponse } from '@angular/common/http';

import { environment } from '../environments/environment';

/** Base URL of the backend API; set per build in src/environments. */
export const API_URL = environment.apiUrl;

const REQUEST_ID = /^[A-Za-z0-9._-]{8,64}$/;

/**
 * Pulls the `{"error": "..."}` message out of an API error response. For a
 * server error it appends the request ID the server logged the failure under,
 * so a report ("reference a8f4...") can be matched to the exact log line.
 */
export function apiErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof HttpErrorResponse) {
    if (err.status === 0) return "Can't reach the server right now.";
    const message = err.error?.error;
    const id = err.error?.request_id;
    const ref = err.status >= 500 && typeof id === 'string' && REQUEST_ID.test(id) ? ` (reference ${id})` : '';
    // Server messages are lower-case sentences ("you can submit up to 5...").
    if (typeof message === 'string' && message) return message.charAt(0).toUpperCase() + message.slice(1) + ref;
    return fallback + ref;
  }
  return fallback;
}
