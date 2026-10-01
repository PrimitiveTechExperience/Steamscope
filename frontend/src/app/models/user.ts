import { Game } from './game';

export interface User {
    user_id: number;
    username: string;
    email: string;
    steam_id: string | null;
    is_admin: boolean;
    created_at: string;
}

export interface Preferences {
    theme: 'light' | 'dark';
    notify_price_drops: boolean;
    price_drop_threshold_percent: number;
    preferred_genres: string[];
}

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

export interface RecentSearch {
    search?: string;
    genres?: string[];
    tags?: string[];
    languages?: string[];
    developers?: string[];
    publishers?: string[];
    min_price?: number;
    max_price?: number;
}
