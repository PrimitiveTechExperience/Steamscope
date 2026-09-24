import { Directive, ElementRef, NgZone, PLATFORM_ID, inject } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

/**
 * Rotates the host element to "face" the cursor while hovered, using a
 * lightweight perspective tilt driven by pointer position relative to the
 * element's center.
 *
 * Native addEventListener (not @HostListener) run via runOutsideAngular,
 * since mousemove fires dozens of times a second and this only ever writes
 * an inline style - there's nothing here Angular's change detection needs
 * to know about.
 */
@Directive({
  selector: '[appTilt]',
  host: { style: 'transform-style: preserve-3d; will-change: transform; transition: transform 0.15s ease;' },
})
export class TiltDirective {
  private el = inject(ElementRef<HTMLElement>);
  private zone = inject(NgZone);
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));
  private maxTilt = 10;

  constructor() {
    if (!this.isBrowser) return;

    this.zone.runOutsideAngular(() => {
      const element = this.el.nativeElement;

      element.addEventListener('mousemove', (event: MouseEvent) => {
        const rect = element.getBoundingClientRect();
        const x = (event.clientX - rect.left) / rect.width - 0.5;
        const y = (event.clientY - rect.top) / rect.height - 0.5;
        const rotateY = x * this.maxTilt * 2;
        const rotateX = -y * this.maxTilt * 2;
        element.style.transform = `perspective(700px) rotateX(${rotateX}deg) rotateY(${rotateY}deg) scale(1.02)`;
      });

      element.addEventListener('mouseleave', () => {
        element.style.transform = 'perspective(700px) rotateX(0deg) rotateY(0deg) scale(1)';
      });
    });
  }
}
