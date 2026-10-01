import { SubmissionStatus } from './user';

export interface AdminUser {
    user_id: number;
    username: string;
    email: string;
    steam_id: string | null;
    is_admin: boolean;
    is_banned: boolean;
    submissions_blocked: boolean;
    created_at: string;
}

export interface AdminItem {
    kind: 'app' | 'bundle';
    id: number;
    name: string | null;
    status: SubmissionStatus;
    submitted_by: string | null;
    created_at: string;
}

export interface GameCount {
    app_id: number;
    name: string;
    count: number;
}

export interface UserCount {
    username: string;
    count: number;
}

export interface DayActivity {
    date: string;
    signups: number;
    watch_adds: number;
    submissions: number;
}

export interface AdminStats {
    users: number;
    new_users_7d: number;
    banned_users: number;
    tracked_games: number;
    tracked_bundles: number;
    awaiting_approval: number;
    watchlist_entries: number;
    pinned_entries: number;
    blacklist_rules: number;
    users_with_watchlist: number;
    most_watched: GameCount[];
    most_pinned: GameCount[];
    top_submitters: UserCount[];
    top_watchers: UserCount[];
    activity: DayActivity[];
    recent_users: AdminUser[];
    recent_items: AdminItem[];
}

export type BlacklistField = 'app_id' | 'name' | 'developer' | 'publisher';

export interface BlacklistRule {
    rule_id: number;
    field: BlacklistField;
    pattern: string;
    note: string;
    created_by: string | null;
    created_at: string;
}

export interface BlacklistMatch {
    app_id: number;
    name: string;
}
