import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

import { API_URL } from '../api';
import { AdminItem, AdminUser } from '../models/admin';

@Injectable({ providedIn: 'root' })
export class AdminService {
  private http = inject(HttpClient);
  private base = `${API_URL}/admin`;

  getUsers(): Observable<AdminUser[]> {
    return this.http.get<AdminUser[]>(`${this.base}/users`);
  }

  deleteUser(userId: number): Observable<void> {
    return this.http.delete<void>(`${this.base}/users/${userId}`);
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
}
