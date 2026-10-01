import { Component } from '@angular/core';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';

/** Frame for every admin page: title and section tabs. */
@Component({
  selector: 'app-admin-shell',
  imports: [RouterLink, RouterLinkActive, RouterOutlet],
  template: `
    <div class="mx-auto max-w-6xl px-6 py-10">
      <h1 class="text-3xl font-black uppercase tracking-tight text-[var(--color-text)]" style="font-family: var(--font-display);">
        Admin
      </h1>
      <nav class="mt-4 flex flex-wrap gap-1 border-b-2 border-[var(--color-border)] pb-3">
        @for (tab of tabs; track tab.path) {
          <a
            [routerLink]="tab.path"
            routerLinkActive="admin-tab-active"
            [routerLinkActiveOptions]="{ exact: true }"
            class="edge-btn bg-[var(--color-surface)] px-3 py-1.5 text-xs text-[var(--color-text-muted)]"
          >
            {{ tab.label }}
          </a>
        }
      </nav>
      <router-outlet />
    </div>
  `,
  styles: `
    .admin-tab-active {
      background: var(--color-accent);
      color: #fff;
    }
  `,
})
export class AdminShellComponent {
  protected tabs = [
    { path: '/admin', label: 'Dashboard' },
    { path: '/admin/games', label: 'Games & approvals' },
    { path: '/admin/users', label: 'Users' },
    { path: '/admin/blacklist', label: 'Blacklist' },
  ];
}
