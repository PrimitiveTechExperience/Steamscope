import { Injectable, PLATFORM_ID, inject, signal } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

export type Theme = 'light' | 'dark';

@Injectable({ providedIn: 'root' })
export class ThemeService {
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));
  readonly theme = signal<Theme>('dark');

  constructor() {
    if (this.isBrowser) {
      this.set(localStorage.getItem('theme') === 'light' ? 'light' : 'dark');
    }
  }

  set(theme: Theme) {
    this.theme.set(theme);
    if (this.isBrowser) {
      document.documentElement.setAttribute('data-theme', theme);
      localStorage.setItem('theme', theme);
    }
  }
}
