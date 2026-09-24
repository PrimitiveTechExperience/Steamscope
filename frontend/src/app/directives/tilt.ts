import { Directive, ElementRef, HostListener, inject } from '@angular/core';

/**
 * Rotates the host element to "face" the cursor while hovered, using a
 * lightweight perspective tilt driven by pointer position relative to the
 * element's center. Pure event-driven (no-op on the server since the events
 * never fire during SSR).
 */
@Directive({
  selector: '[appTilt]',
  host: { style: 'transform-style: preserve-3d; will-change: transform; transition: transform 0.15s ease;' },
})
export class TiltDirective {
  private el = inject(ElementRef<HTMLElement>);
  private maxTilt = 10;

  @HostListener('mousemove', ['$event'])
  onMouseMove(event: MouseEvent) {
    const rect = this.el.nativeElement.getBoundingClientRect();
    const x = (event.clientX - rect.left) / rect.width - 0.5;
    const y = (event.clientY - rect.top) / rect.height - 0.5;
    const rotateY = x * this.maxTilt * 2;
    const rotateX = -y * this.maxTilt * 2;
    this.el.nativeElement.style.transform =
      `perspective(700px) rotateX(${rotateX}deg) rotateY(${rotateY}deg) scale(1.02)`;
  }

  @HostListener('mouseleave')
  onMouseLeave() {
    this.el.nativeElement.style.transform = 'perspective(700px) rotateX(0deg) rotateY(0deg) scale(1)';
  }
}
