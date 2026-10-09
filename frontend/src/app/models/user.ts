import { Game } from './game';

export interface User {
    user_id: number;
    username: string;
    email: string;
    steam_id: string | null;
    is_admin: boolean;
    submissions_blocked: boolean;
    created_at: string;
}

export interface Preferences {
    theme: 'light' | 'dark';
    notify_price_drops: boolean;
    price_drop_threshold_percent: number;
    preferred_genres: string[];
    /** Target-price alerts: send one by email, to a Discord webhook, or as a browser push. */
    alert_email: boolean;
    alert_discord: boolean;
    discord_webhook_url: string;
    alert_push: boolean;
}

/** Which alert channels this server can send through. */
export interface AlertChannels {
    email: boolean;
    discord: boolean;
    push: boolean;
    /** The key a browser needs to subscribe to push; empty when push is not set up. */
    vapid_public_key: string;
}

export type AlertChannel = 'email' | 'discord' | 'push';

export interface WatchedGame {
    game: Game;
    pinned: boolean;
    target_price: number | null;
    watched_at: string;
}

export type NotificationKind =
  | 'price_drop'
  | 'target_price'
  | 'submission_tracked'
  | 'submission_failed'
  | 'submission_rejected'
  | 'bundle_tracked'
  | 'bundle_failed';

export interface AppNotification {
    notification_id: number;
    app_id: number | null;
    kind: NotificationKind;
    message: string;
    read_at: string | null;
    created_at: string;
}

export type SubmissionStatus = 'awaiting_approval' | 'pending' | 'tracked' | 'failed' | 'rejected';

export interface Submission {
    kind: 'app' | 'bundle';
    id: number;
    name: string | null;
    status: SubmissionStatus;
    created_at: string;
}

export interface Deal {
    game: Game;
    average_price: number;
    percent_below_usual: number;
    watched: boolean;
}

export interface Feed {
    watchlist: WatchedGame[];
    deals: Deal[];
    suggestions: Game[];
}

export interface SteamPlayedGame {
    app_id: number;
    name: string;
    icon_url: string;
    playtime_2weeks: number;
    playtime_forever: number;
    /** Our tracking status for this game; empty when it was never submitted. */
    track_status: SubmissionStatus | '';
}

export interface SteamProfile {
    steam_id: string;
    persona_name: string;
    avatar_url: string;
    profile_url: string;
    status: string;
    currently_playing: string;
    recently_played: SteamPlayedGame[];
}

/** A wishlist game we do not track yet; name is "" when Steam could not be asked. */
export interface WishlistGame {
    app_id: number;
    name: string;
}

/**
 * Where a wishlist stands: "empty" (nothing on it, or private), "incomplete"
 * (games still to import), "waiting" (everything else is in; requested games are
 * being added) or "complete" (every game is on the watchlist).
 */
export type WishlistState = 'empty' | 'incomplete' | 'waiting' | 'complete';

export interface WishlistStatus {
    state: WishlistState;
    wishlist_size: number;
    /** Games that still need importing or requesting. */
    remaining: number;
    /** Requested games still being added. */
    waiting: number;
    /** Games that cannot be added (turned down or blocked). */
    unavailable: number;
}

export interface WishlistImportResult {
    /** 0 means the wishlist is empty or private. */
    wishlist_size: number;
    /** Games the user started watching just now. */
    watched: number;
    already_watched: number;
    requestable: WishlistGame[];
    awaiting_review: number;
    unavailable: number;
    state: WishlistState;
}

export interface WishlistRequestResult {
    state: WishlistState;
    requested: number;
    skipped: number;
    /** "pending" when the games are being added straight away (admins), else "awaiting_approval". */
    status: 'pending' | 'awaiting_approval';
}

export interface RecentSearch {
    search?: string;
    genres?: string[];
    tags?: string[];
    languages?: string[];
    developers?: string[];
    publishers?: string[];
    min_price?: number;
    max_price?: number;
    /** "At least this % off", 1-100. */
    min_discount?: number;
}
