import { computed, signal } from '@angular/core';

import { Game } from '../app/models/game';
import { Bundle, BundleDetail, BundleValue } from '../app/models/bundle';
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
    at_record_low: false,
    game_count: 3,
    games: [
      { app_id: 1, name: 'On The Site', tracked: true, track_status: '', price: 12, regular_price: 20 },
      { app_id: 2, name: 'Not On The Site', tracked: false, track_status: '', price: 12, regular_price: 20 },
      { app_id: 3, name: 'Already Requested', tracked: false, track_status: 'awaiting_approval', price: 12, regular_price: 20 },
    ],
    updated_at: '2026-10-01T00:00:00Z',
    price_history: [],
    record_low: 0,
    history_days: 0,
    value: makeBundleValue(),
    ...over,
  };
}

/** A realistic assessment: $20 bundle of three $20 games on sale to $12 each. */
export function makeBundleValue(over: Partial<BundleValue> = {}): BundleValue {
  return {
    verdict: 'great_deal',
    score: 85,
    reasons: [
      { code: 'cheaper_than_separate', text: 'Buying these games separately today costs $36.00; the bundle is $20.00, saving $16.00 (44%)', impact: 30 },
      { code: 'deep_vs_regular', text: '67% below the games\' regular prices ($60.00)', impact: 15 },
      { code: 'above_bundle_record_low', text: '25% above the lowest price recorded for this bundle ($16.00)', impact: -10 },
    ],
    totals: { regular: 60, separate: 36, bundle: 20, priced_items: 3, items: 3 },
    savings_vs_separate: 16,
    savings_vs_separate_percent: 44.44,
    savings_vs_regular: 40,
    savings_vs_regular_percent: 66.67,
    completeness: 1,
    at_record_low: false,
    record_low: 16,
    cheaper_alone_count: 0,
    items: [1, 2, 3].map((id) => ({
      app_id: id, name: `Game ${id}`, price: 12, regular_price: 20, discount_percent: 40, bundle_share: 6.67, cheaper_alone: false, priced: true,
    })),
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
