import { Component, inject, PLATFORM_ID, signal } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { RouterOutlet, RouterLink } from '@angular/router';

@Component({
  selector: 'app-root',
  imports: [RouterOutlet, RouterLink],
  templateUrl: './app.html',
  styleUrl: './app.css'
})
export class App {
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));
  protected theme = signal<'light' | 'dark'>('light');

  constructor() {
    if (this.isBrowser) {
      const initial = localStorage.getItem('theme') === 'dark' ? 'dark' : 'light';
      this.theme.set(initial);
      document.documentElement.setAttribute('data-theme', initial);
    }
  }

  toggleTheme() {
    if (!this.isBrowser) return;
    const next = this.theme() === 'dark' ? 'light' : 'dark';
    this.theme.set(next);
    document.documentElement.setAttribute('data-theme', next);
    localStorage.setItem('theme', next);
  }
}
