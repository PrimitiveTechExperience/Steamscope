import { Directive, ElementRef, PLATFORM_ID, inject, afterNextRender } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

/**
 * Adds `reveal-on-scroll` immediately and `is-visible` once the host enters
 * the viewport (see styles.css for the actual ease-in transition). Browser
 * only - IntersectionObserver doesn't exist during SSR.
 */
@Directive({
  selector: '[appRevealOnScroll]',
  host: { class: 'reveal-on-scroll' },
})
export class RevealOnScrollDirective {
  private el = inject(ElementRef<HTMLElement>);
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  constructor() {
    afterNextRender(() => {
      if (!this.isBrowser) return;

      const element = this.el.nativeElement;
      const observer = new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (entry.isIntersecting) {
              element.classList.add('is-visible');
              observer.unobserve(element);
            }
          }
        },
        { threshold: 0.15 }
      );
      observer.observe(element);
    });
  }
}
