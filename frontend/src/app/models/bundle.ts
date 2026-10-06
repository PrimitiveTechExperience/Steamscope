import { PricePoint } from './game';
import { SubmissionStatus } from './user';

export interface BundleGame {
    app_id: number;
    name: string;
    /** True when the game has its own page on Steamscope. */
    tracked: boolean;
    /** Status of a request to add this game, when it isn't on the site yet. */
    track_status: SubmissionStatus | '';
    /** Current and regular (undiscounted) price in USD; 0 when unknown. */
    price: number;
    regular_price: number;
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
    /** On sale at the lowest price recorded for this bundle. */
    at_record_low: boolean;
    game_count: number;
    games: BundleGame[] | null;
    updated_at: string;
}

export type BundleVerdict = 'great_deal' | 'good_deal' | 'fair' | 'poor_value' | 'not_enough_data';

export interface BundleReason {
    code: string;
    text: string;
    impact: number;
}

export interface BundleItemValue {
    app_id: number;
    name: string;
    price: number;
    regular_price: number;
    discount_percent: number;
    /** The part of the bundle price this game accounts for, split by regular price. */
    bundle_share: number;
    /** Buying just this game now costs less than its share of the bundle. */
    cheaper_alone: boolean;
    priced: boolean;
}

export interface BundleValue {
    verdict: BundleVerdict;
    /** 0-100: high means a good deal. */
    score: number;
    reasons: BundleReason[];
    totals: { regular: number; separate: number; bundle: number; priced_items: number; items: number };
    /** Negative when the bundle costs more. */
    savings_vs_separate: number;
    savings_vs_separate_percent: number;
    savings_vs_regular: number;
    savings_vs_regular_percent: number;
    /** Share (0-1) of games whose price was known. */
    completeness: number;
    at_record_low: boolean;
    record_low: number;
    cheaper_alone_count: number;
    items: BundleItemValue[];
}

export interface BundleDetail extends Bundle {
    price_history: PricePoint[] | null;
    /** Lowest bundle price recorded, and how many days that record spans. */
    record_low: number;
    history_days: number;
    value: BundleValue;
}
