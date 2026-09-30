const FALLBACK_MARKER = 'data-fallback-applied';

/** Steam's header image sometimes fails to load (stale CDN token, scrape gap).
 * Fall back once to the stable, non-token CDN path before giving up. */
export function onHeaderImageError(event: Event, appId: number): void {
  const img = event.target as HTMLImageElement;
  if (img.getAttribute(FALLBACK_MARKER)) return;
  img.setAttribute(FALLBACK_MARKER, 'true');
  img.src = `https://cdn.cloudflare.steamstatic.com/steam/apps/${appId}/header.jpg`;
}
