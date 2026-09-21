create table games (
    app_id integer primary key,

    name text not null,
    url text not null,
    description text not null,
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
