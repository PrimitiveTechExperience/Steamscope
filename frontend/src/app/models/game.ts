export interface Game{
    app_id: number;
    name: string;
    url: string;
    description: string;
    header_image: string;
    release_date: string;
    price: number;
    original_price: number;
    discount_percentage: number;
    review_score: string;
    review_count: number;
    windows_compatible: boolean;
    mac_compatible: boolean;
    linux_compatible: boolean;
    developers: string[];
    publishers: string[];
    genres: string[];
    tags: string[];
    supported_languages: string[];
    reviews: Review[];
}

export interface Review{
    app_id: number;
    recommendation_id: string;
    steam_id: string;
    language: string;
    review: string;
    voted_up: boolean;
    timestamp_created: string;
    timestamp_updated: string;
    playtime_forever: number;
    playtime_at_review: number;
    helpful_votes: number;
    funny_votes: number;
}

export interface GamesResponse{
    games: Game[];
    total: number;
    limit: number;
    offset: number;
}

export interface PricePoint{
    date: string;
    price: number;
    original_price: number;
    discount_percentage: number;
}

export interface FilterOptions{
    genres: string[];
    tags: string[];
    developers: string[];
    publishers: string[];
    languages: string[];
}