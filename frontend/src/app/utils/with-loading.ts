import { Observable, map, startWith } from 'rxjs';

export interface LoadingState<T> {
  data: T;
  loading: boolean;
}

/**
 * Tags a stream with a loading flag: `fallback` immediately with
 * `loading: true`, then the real value with `loading: false` once it
 * arrives. Lets a section show its own skeleton instead of relying on one
 * global "something, somewhere is loading" indicator.
 *
 * Apply this to a stream that has already handled its own errors (e.g. via
 * catchError upstream) - it only adds the loading flag, nothing else.
 */
export function withLoading<T>(source$: Observable<T>, fallback: T): Observable<LoadingState<T>> {
  return source$.pipe(
    map((data) => ({ data, loading: false })),
    startWith({ data: fallback, loading: true })
  );
}
