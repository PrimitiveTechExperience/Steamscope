import { Directive, ElementRef, PLATFORM_ID, inject, afterNextRender } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

const THRESHOLDS = Array.from({ length: 21 }, (_, i) => i / 20);

/**
 * Unlike RevealOnScrollDirective (one-shot fade-in), this continuously
 * tracks how much of the host is visible and writes it to --intersect, so
 * an element can grow/shrink as it scrolls through the viewport rather than
 * just appearing once. Browser only - IntersectionObserver doesn't run
 * during SSR.
 */
@Directive({
  selector: '[appGrowOnScroll]',
  host: { class: 'grow-on-scroll' },
})
export class GrowOnScrollDirective {
  private el = inject(ElementRef<HTMLElement>);
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  constructor() {
    afterNextRender(() => {
      if (!this.isBrowser) return;

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
  }
}
