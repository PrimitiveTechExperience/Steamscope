import { Component, computed, input, signal } from '@angular/core';

import { Bundle } from '../../models/bundle';

/**
 * A bundle's cover. Uses Steam's header image when there is one; otherwise
 * (or if it fails to load) builds a collage from the header images of the
 * games inside the bundle.
 */
@Component({
  selector: 'app-bundle-art',
  templateUrl: './bundle-art.html',
})
export class BundleArtComponent {
  bundle = input.required<Bundle>();

  private imageFailed = signal(false);
  private failedTiles = signal<ReadonlySet<number>>(new Set());

  protected showHeader = computed(() => !!this.bundle().header_image && !this.imageFailed());

  protected tiles = computed(() =>
    (this.bundle().games ?? [])
      .slice(0, 4)
      .filter((g) => !this.failedTiles().has(g.app_id))
      .map((g) => ({
        appId: g.app_id,
        name: g.name,
        src: `https://cdn.akamai.steamstatic.com/steam/apps/${g.app_id}/header.jpg`,
      }))
  );

  protected onHeaderError() {
    this.imageFailed.set(true);
  }

  protected onTileError(appId: number) {
    this.failedTiles.update((set) => new Set(set).add(appId));
  }
}
