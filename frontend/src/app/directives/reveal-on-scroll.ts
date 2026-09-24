import { Directive, ElementRef, NgZone, PLATFORM_ID, inject, afterNextRender } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

/**
 * Adds `reveal-on-scroll` immediately and `is-visible` once the host enters
 * the viewport (see styles.css for the actual ease-in transition). Browser
 * only - IntersectionObserver doesn't exist during SSR. Runs outside NgZone
 * since it's a one-off DOM class toggle Angular's bindings never need to
 * react to.
 */
@Directive({
  selector: '[appRevealOnScroll]',
  host: { class: 'reveal-on-scroll' },
})
export class RevealOnScrollDirective {
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
    });
  }
}
