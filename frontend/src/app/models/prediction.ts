export interface CurvePoint {
    date: string;
    days_ahead: number;
    expected_price: number;
    /** Chance (0-1) a sale is running on this date. */
    p_on_sale: number;
    /** Chance (0-1) the price has been at least 5% below today's at some point by this date. */
    p_lower_by: number;
}

export interface Horizon {
    days: number;
    p_lower: number;
    expected_price: number;
    expected_low: number;
}

export interface TypicalSale {
    count: number;
    median_depth_percent: number;
    median_price: number;
    median_duration_days: number;
    median_interval_days: number | null;
}

export interface NextSale {
    p25_days: number | null;
    median_days: number | null;
    p75_days: number | null;
}

export type ForecastModel = 'weibull_renewal' | 'poisson' | 'no_sales_seen' | 'insufficient' | 'free';

export interface Forecast {
    app_id: number;
    generated_at: string;
    model: ForecastModel;
    current_price: number;
    regular_price: number;
    current_discount_percent: number;
    on_sale: boolean;
    history_days: number;
    historic_low: number;
    /** On sale at the lowest price on record. */
    at_record_low: boolean;
    used_extended_history: boolean;
    typical_sale: TypicalSale;
    next_sale: NextSale;
    /** Chance (0-100) of a lower price within 180 days. */
    score: number;
    confidence: number;
    curve: CurvePoint[];
    horizons: Horizon[];
    cached: boolean;
}

export type Verdict = 'buy_now' | 'wait' | 'toss_up' | 'not_enough_data' | 'free';

export interface AdviceReason {
    code: string;
    text: string;
    /** Points added to (or, when negative, taken from) the buy-now score. */
    impact: number;
}

export interface Advice {
    verdict: Verdict;
    /** 0-100: high means buy now, low means wait. */
    score: number;
    confidence: number;
    reasons: AdviceReason[];
    patience_days: number;
    chance_of_lower: number;
    expected_saving_percent: number;
    at_record_low: boolean;
    wait_until: string | null;
    personalized: boolean;
    generated_at: string;
    cached: boolean;
}
