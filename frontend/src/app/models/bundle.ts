import { PricePoint } from './game';
import { SubmissionStatus } from './user';

export interface BundleGame {
    app_id: number;
    name: string;
    /** True when the game has its own page on Steamscope. */
    tracked: boolean;
    /** Status of a request to add this game, when it isn't on the site yet. */
    track_status: SubmissionStatus | '';
}

export interface Bundle {
    bundle_id: number;
    name: string;
    url: string;
    header_image: string;
    price: number;
    original_price: number;
    discount_percentage: number;
    status: string;
    game_count: number;
    games: BundleGame[] | null;
    updated_at: string;
}

export interface BundleDetail extends Bundle {
    price_history: PricePoint[] | null;
}
