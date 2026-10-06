create table games (
    app_id integer primary key,

    name text not null,
    url text not null,
    description text not null,
    description_html text not null default '',
    header_image text not null default '',

    release_date date not null,

    price numeric(10, 2) not null,
    original_price numeric(10, 2) not null,
    discount_percentage decimal(5, 2) not null,

    review_score integer not null,
    review_count integer not null,

    windows_compatible boolean not null,
    mac_compatible boolean not null,
    linux_compatible boolean not null,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

create table reviews (
    recommendation_id text primary key,
    app_id integer not null references games(app_id) on delete cascade,

    steam_id text not null,
    author_name text not null default '',
    author_avatar text not null default '',
    num_games_owned integer not null default 0,
    num_reviews integer not null default 0,
    language text,
    review text,
    voted_up boolean not null,

    timestamp_created timestamptz not null,
    timestamp_updated timestamptz not null,

    playtime_forever integer not null,
    playtime_at_review integer not null,

    helpful_votes integer not null,
    funny_votes integer not null
);

create table developers (
    developer_id bigint generated always as identity primary key,
    developer text not null unique
);

create table game_developers (
    app_id integer not null references games(app_id) on delete cascade,
    developer_id bigint not null references developers(developer_id) on delete cascade,

    primary key (app_id, developer_id)
);

create table publishers (
    publisher_id bigint generated always as identity primary key,
    publisher text not null unique
);

create table game_publishers (
    app_id integer not null references games(app_id) on delete cascade,
    publisher_id bigint not null references publishers(publisher_id) on delete cascade,

    primary key (app_id, publisher_id)
);

create table genres (
    genre_id bigint generated always as identity primary key,
    genre text not null unique
);

create table game_genres (
    app_id integer not null references games(app_id) on delete cascade,
    genre_id bigint not null references genres(genre_id) on delete cascade,

    primary key (app_id, genre_id)
);

create table tags (
    tag_id bigint generated always as identity primary key,
    tag text not null unique
);

create table game_tags (
    app_id integer not null references games(app_id) on delete cascade,
    tag_id bigint not null references tags(tag_id) on delete cascade,

    primary key (app_id, tag_id)
);

create table languages (
    language_id bigint generated always as identity primary key,
    language text not null unique
);

create table game_languages (
    app_id integer not null references games(app_id) on delete cascade,
    language_id bigint not null references languages(language_id) on delete cascade,

    primary key (app_id, language_id)
);

create index idx_review_app_id on reviews(app_id);
create index idx_review_steam_id on reviews(steam_id);
create index idx_review_timestamp_created on reviews(timestamp_created);

create table price_history (
    id bigint generated always as identity primary key,
    app_id integer not null references games(app_id) on delete cascade,
    recorded_date date not null,

    price numeric(10, 2) not null,
    original_price numeric(10, 2) not null,
    discount_percentage decimal(5, 2) not null,

    created_at timestamptz not null default now(),

    unique (app_id, recorded_date)
);

create index idx_price_history_app_id on price_history(app_id);
create index idx_price_history_recorded_date on price_history(recorded_date);

create table users (
    user_id bigint generated always as identity primary key,
    username text not null,
    email text not null,
    password_hash text not null,
    steam_id text unique,
    is_admin boolean not null default false,
    is_banned boolean not null default false,
    submissions_blocked boolean not null default false,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create unique index idx_users_username_lower on users (lower(username));
create unique index idx_users_email_lower on users (lower(email));

create table user_preferences (
    user_id bigint primary key references users(user_id) on delete cascade,
    theme text not null default 'dark' check (theme in ('light', 'dark')),
    notify_price_drops boolean not null default true,
    price_drop_threshold_percent integer not null default 10 check (price_drop_threshold_percent between 1 and 100),
    preferred_genres text[] not null default '{}'
);

create table watched_games (
    user_id bigint not null references users(user_id) on delete cascade,
    app_id integer not null references games(app_id) on delete cascade,
    pinned boolean not null default false,
    target_price numeric(10, 2),
    created_at timestamptz not null default now(),

    primary key (user_id, app_id)
);

create index idx_watched_games_app_id on watched_games(app_id);

create table notifications (
    notification_id bigint generated always as identity primary key,
    user_id bigint not null references users(user_id) on delete cascade,
    app_id integer references games(app_id) on delete cascade,
    kind text not null check (kind in ('price_drop', 'target_price', 'submission_tracked', 'submission_failed', 'submission_rejected', 'bundle_tracked', 'bundle_failed')),
    message text not null,
    read_at timestamptz,
    created_at timestamptz not null default now()
);

create index idx_notifications_user_created on notifications(user_id, created_at desc);

-- Source of truth for what the scheduler scrapes. No FK to games: a row
-- exists before the game's first successful scrape.
create table tracked_games (
    app_id integer primary key,
    submitted_by bigint references users(user_id) on delete set null,
    status text not null default 'pending' check (status in ('awaiting_approval', 'pending', 'tracked', 'failed', 'rejected')),
    created_at timestamptz not null default now()
);

create table bundles (
    bundle_id integer primary key,
    name text not null default '',
    url text not null,
    header_image text not null default '',
    price numeric(10, 2) not null default 0,
    original_price numeric(10, 2) not null default 0,
    discount_percentage integer not null default 0,
    status text not null default 'pending' check (status in ('awaiting_approval', 'pending', 'tracked', 'failed', 'rejected')),
    submitted_by bigint references users(user_id) on delete set null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

-- No FK on app_id: a bundle can include games we don't track.
create table bundle_games (
    bundle_id integer not null references bundles(bundle_id) on delete cascade,
    app_id integer not null,
    name text not null default '',
    -- What the bundle page showed for this game (USD; 0 = unknown): its current
    -- price and its regular, undiscounted price.
    price numeric(10, 2) not null default 0,
    regular_price numeric(10, 2) not null default 0,

    primary key (bundle_id, app_id)
);

create index idx_bundle_games_app_id on bundle_games(app_id);

create table bundle_price_history (
    bundle_id integer not null references bundles(bundle_id) on delete cascade,
    recorded_date date not null,
    price numeric(10, 2) not null,
    original_price numeric(10, 2) not null,
    discount_percentage integer not null,

    primary key (bundle_id, recorded_date)
);

-- Admin-managed rules that keep games off the site. field 'app_id' blocks one
-- app ID (pattern is the number); the others are case-insensitive regular
-- expressions matched against the game's name, a developer or a publisher.
create table if not exists blacklist_rules (
    rule_id bigint generated always as identity primary key,
    field text not null check (field in ('app_id', 'name', 'developer', 'publisher')),
    pattern text not null check (length(pattern) between 1 and 200),
    note text not null default '' check (length(note) <= 200),
    created_by bigint references users(user_id) on delete set null,
    created_at timestamptz not null default now(),

    unique (field, pattern)
);

-- Row level security: every table has RLS enabled with no policies, so
-- Supabase's public API roles (anon / authenticated) can read or write
-- nothing. The Go backend connects as the postgres role, which bypasses
-- RLS, so it is unaffected. Add explicit policies here if you ever expose
-- tables through the Supabase client directly.
do $$
declare t record;
begin
    for t in select tablename from pg_tables where schemaname = 'public' loop
        execute format('alter table public.%I enable row level security', t.tablename);
    end loop;
end $$;
