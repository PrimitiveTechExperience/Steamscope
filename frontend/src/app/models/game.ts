export interface Game{
    app_id: number;
    name: string;
    url: string;
    description: string;
    release_date: string;
    price: number;
    original_price: number;
    discount_percentage: number;
    review_score: number;
    review_count: number;
    windows_compatible: boolean;
    mac_compatible: boolean;
    linux_compatible: boolean;
    developers: string[];
    publishers: string[];
    genres: string[];
    tags: string[];
    languages: string[];
}

export interface GamesResponse{
    games: Game[];
    total: number;
    limit: number;
    offset: number;
}