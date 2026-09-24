import { Directive, ElementRef, NgZone, PLATFORM_ID, inject, afterNextRender } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

// Coarser steps than a true 0-1 ratio would need: 21 thresholds fired a
// callback on every single percent of visibility change, which under
// zone.js meant a full app-wide change detection pass per step per card.
const THRESHOLDS = Array.from({ length: 11 }, (_, i) => i / 10);

/**
 * Unlike RevealOnScrollDirective (one-shot fade-in), this continuously
 * tracks how much of the host is visible and writes it to --intersect, so
 * an element can grow/shrink as it scrolls through the viewport rather than
 * just appearing once. Browser only - IntersectionObserver doesn't run
 * during SSR.
 *
 * Runs outside NgZone: this only ever writes a CSS custom property directly
 * on the element, never touching anything Angular's change detection needs
 * to know about, so there's no reason to let zone.js schedule an app-wide
 * CD cycle on every intersection step while the user scrolls.
 */
@Directive({
  selector: '[appGrowOnScroll]',
  host: { class: 'grow-on-scroll' },
})
export class GrowOnScrollDirective {
  private el = inject(ElementRef<HTMLElement>);
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));
  private zone = inject(NgZone);

  constructor() {
    afterNextRender(() => {
      if (!this.isBrowser) return;

      this.zone.runOutsideAngular(() => {
        const element = this.el.nativeElement;
        const observer = new IntersectionObserver(
          (entries) => {
            for (const entry of entries) {
              element.style.setProperty('--intersect', `${entry.intersectionRatio}`);
            }
          },
          { threshold: THRESHOLDS }
        );
        observer.observe(element);
      });
    });
  }
}
