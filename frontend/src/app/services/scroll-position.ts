import { Injectable, PLATFORM_ID, inject } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

/**
 * Mirrors window.scrollY onto a CSS custom property (--scroll-y) on <html>,
 * rAF-throttled, so background layers can react to scroll purely in CSS
 * without every component needing its own scroll listener.
 */
@Injectable({ providedIn: 'root' })
export class ScrollPositionService {
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));
  private ticking = false;
  private started = false;

  start() {
    if (!this.isBrowser || this.started) return;
    this.started = true;

    const update = () => {
      document.documentElement.style.setProperty('--scroll-y', `${window.scrollY}`);
      this.ticking = false;
    };

    window.addEventListener(
      'scroll',
      () => {
        if (!this.ticking) {
          this.ticking = true;
          requestAnimationFrame(update);
        }
      },
      { passive: true }
    );

    update();
  }
}
