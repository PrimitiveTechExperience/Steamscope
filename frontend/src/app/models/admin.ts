import { SubmissionStatus } from './user';

export interface AdminUser {
    user_id: number;
    username: string;
    email: string;
    steam_id: string | null;
    is_admin: boolean;
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
