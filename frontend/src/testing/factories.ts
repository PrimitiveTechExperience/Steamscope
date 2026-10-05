import { computed, signal } from '@angular/core';

import { Game } from '../app/models/game';
import { Bundle, BundleDetail } from '../app/models/bundle';
import { User } from '../app/models/user';
import { AdminUser } from '../app/models/admin';

export function makeGame(over: Partial<Game> = {}): Game {
  return {
    app_id: 730,
    name: 'Counter-Strike 2',
    url: 'https://store.steampowered.com/app/730',
    description: 'A <b>tactical</b> shooter.',
    header_image: 'https://cdn.example/730/header.jpg',
    release_date: '2023-09-27T00:00:00Z',
    price: 9.99,
    original_price: 19.99,
    discount_percentage: 50,
    review_score: 'Very Positive',
    review_count: 1000,
    windows_compatible: true,
    mac_compatible: false,
    linux_compatible: true,
    developers: ['Valve'],
    publishers: ['Valve'],
    genres: ['Action'],
    tags: ['FPS'],
    supported_languages: ['English'],
    reviews: [],
    ...over,
  };
}

export function makeBundle(over: Partial<BundleDetail> = {}): BundleDetail {
  return {
    bundle_id: 5001,
    name: 'Starter Pack',
    url: 'https://store.steampowered.com/bundle/5001',
    header_image: 'https://cdn.example/bundle/5001.jpg',
    price: 20,
    original_price: 40,
    discount_percentage: 50,
    status: 'tracked',
    game_count: 3,
    games: [
      { app_id: 1, name: 'On The Site', tracked: true, track_status: '' },
      { app_id: 2, name: 'Not On The Site', tracked: false, track_status: '' },
      { app_id: 3, name: 'Already Requested', tracked: false, track_status: 'awaiting_approval' },
    ],
    updated_at: '2026-10-01T00:00:00Z',
    price_history: [],
    ...over,
  };
}

export function asListedBundle(b: BundleDetail): Bundle {
  const { price_history: _history, ...bundle } = b;
  return bundle;
}

export function makeUser(over: Partial<User> = {}): User {
  return {
    user_id: 1,
    username: 'alice',
    email: 'alice@example.com',
    steam_id: null,
    is_admin: false,
    submissions_blocked: false,
    created_at: '2026-01-01T12:00:00Z',
    ...over,
  };
}

export function makeAdminUser(over: Partial<AdminUser> = {}): AdminUser {
  return {
    user_id: 2,
    username: 'bob',
    email: 'bob@example.com',
    steam_id: null,
    is_admin: false,
    is_banned: false,
    submissions_blocked: false,
    created_at: '2026-02-01T12:00:00Z',
    ...over,
  };
}

/** A stand-in for AuthService with just the surface components read. */
export function fakeAuth(user: User | null) {
  const userSignal = signal<User | null | undefined>(user);
  return {
    user: userSignal,
    isLoggedIn: computed(() => !!userSignal()),
    ready: () => Promise.resolve(userSignal() ?? null),
  };
}

/**
 * The element's visible text for stable assertions: text nodes are joined with
 * single spaces (so adjacent elements don't run together) and whitespace is
 * collapsed.
 */
export function textOf(el: Element | null | undefined): string {
  if (!el) return '';
  const parts: string[] = [];
  const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node; node = walker.nextNode()) parts.push(node.textContent ?? '');
  return parts.join(' ').replace(/\s+/g, ' ').trim();
}

/** jsdom has no IntersectionObserver; the scroll directives need one to exist. */
export function stubIntersectionObserver() {
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  );
}
