import { HttpErrorResponse } from '@angular/common/http';

import { apiErrorMessage } from './api';

describe('apiErrorMessage', () => {
  it('capitalizes the server message', () => {
    expect(apiErrorMessage(new HttpErrorResponse({ status: 429, error: { error: 'too many requests' } }), 'fallback')).toBe('Too many requests');
  });

  it('adds the request id to a server error so it can be quoted when reporting it', () => {
    const err = new HttpErrorResponse({ status: 500, error: { error: 'failed to load bundle', request_id: 'a8f41ca8efafde2a' } });
    expect(apiErrorMessage(err, 'fallback')).toBe('Failed to load bundle (reference a8f41ca8efafde2a)');
  });

  it('adds the id to the fallback when the 500 has no message', () => {
    const err = new HttpErrorResponse({ status: 500, error: { request_id: 'abc12345' } });
    expect(apiErrorMessage(err, 'Something went wrong')).toBe('Something went wrong (reference abc12345)');
  });

  it('does not add one to client errors', () => {
    const err = new HttpErrorResponse({ status: 400, error: { error: 'invalid id', request_id: 'abc12345' } });
    expect(apiErrorMessage(err, 'fallback')).toBe('Invalid id');
  });

  it('ignores a request id that is not a plain string of safe characters', () => {
    const err = new HttpErrorResponse({ status: 500, error: { error: 'failed', request_id: '<script>' } });
    expect(apiErrorMessage(err, 'fallback')).toBe('Failed');
  });

  it('reports an unreachable server and uses the fallback otherwise', () => {
    expect(apiErrorMessage(new HttpErrorResponse({ status: 0 }), 'x')).toBe("Can't reach the server right now.");
    expect(apiErrorMessage(new Error('boom'), 'x')).toBe('x');
  });
});
