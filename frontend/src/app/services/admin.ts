import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';

import { API_URL } from '../api';
import { AdminItem, AdminStats, AdminUser, AuditAction, AuditEntry, BlacklistField, BlacklistMatch, BlacklistRule } from '../models/admin';

@Injectable({ providedIn: 'root' })
export class AdminService {
  private http = inject(HttpClient);
  private base = `${API_URL}/admin`;

  getStats(): Observable<AdminStats> {
    return this.http.get<AdminStats>(`${this.base}/stats`);
  }

  getUsers(): Observable<AdminUser[]> {
    return this.http.get<AdminUser[]>(`${this.base}/users`);
  }

  deleteUser(userId: number): Observable<void> {
    return this.http.delete<void>(`${this.base}/users/${userId}`);
  }

  moderateUser(userId: number, change: { is_banned?: boolean; submissions_blocked?: boolean }): Observable<void> {
    return this.http.put<void>(`${this.base}/users/${userId}/moderation`, change);
  }

  getItems(): Observable<AdminItem[]> {
    return this.http.get<AdminItem[]>(`${this.base}/items`);
  }

  deleteItem(item: AdminItem): Observable<void> {
    return this.http.delete<void>(`${this.base}/items/${item.kind}/${item.id}`);
  }

  approveItem(item: AdminItem): Observable<void> {
    return this.http.post<void>(`${this.base}/items/${item.kind}/${item.id}/approve`, {});
  }

  rejectItem(item: AdminItem): Observable<void> {
    return this.http.post<void>(`${this.base}/items/${item.kind}/${item.id}/reject`, {});
  }

  getBlacklist(): Observable<BlacklistRule[]> {
    return this.http.get<BlacklistRule[]>(`${this.base}/blacklist`);
  }

  addBlacklistRule(rule: { field: BlacklistField; pattern: string; note: string }): Observable<BlacklistRule> {
    return this.http.post<BlacklistRule>(`${this.base}/blacklist`, rule);
  }

  deleteBlacklistRule(id: number): Observable<void> {
    return this.http.delete<void>(`${this.base}/blacklist/${id}`);
  }

  getBlacklistMatches(id: number): Observable<BlacklistMatch[]> {
    return this.http.get<BlacklistMatch[]>(`${this.base}/blacklist/${id}/matches`);
  }

  purgeBlacklistMatches(id: number): Observable<{ removed: number }> {
    return this.http.post<{ removed: number }>(`${this.base}/blacklist/${id}/purge`, {});
  }

  /** Newest first. before is an audit_id: entries older than it. */
  getAuditLog(query: { action?: AuditAction | ''; before?: number; limit?: number } = {}): Observable<AuditEntry[]> {
    let params = new HttpParams();
    if (query.action) params = params.set('action', query.action);
    if (query.before) params = params.set('before', query.before);
    if (query.limit) params = params.set('limit', query.limit);
    return this.http.get<AuditEntry[]>(`${this.base}/audit`, { params });
  }
}
