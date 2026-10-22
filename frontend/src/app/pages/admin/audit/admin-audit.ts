import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { AdminService } from '../../../services/admin';
import { AuditAction, AuditEntry } from '../../../models/admin';
import { apiErrorMessage } from '../../../api';

const PAGE_SIZE = 50;

export const ACTION_LABELS: Record<AuditAction, string> = {
  'user.ban': 'Ban',
  'user.unban': 'Unban',
  'user.block_submissions': 'Block submissions',
  'user.unblock_submissions': 'Allow submissions',
  'user.delete': 'Delete user',
  'item.delete': 'Delete game/bundle',
  'item.approve': 'Approve submission',
  'item.reject': 'Reject submission',
  'blacklist.add': 'Add blacklist rule',
  'blacklist.delete': 'Remove blacklist rule',
  'blacklist.purge': 'Purge blacklist matches',
};

const FIELD_NAMES: Record<string, string> = { app_id: 'App ID', name: 'name', developer: 'developer', publisher: 'publisher' };

function str(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

/** One line saying what happened, from the entry's details. */
export function describeAudit(e: AuditEntry): string {
  const d = e.details ?? {};
  const who = str(d['username']) || `user #${e.target_id}`;
  const thing = e.target_type === 'bundle' ? 'bundle' : 'game';
  const named = str(d['name']) ? `"${str(d['name'])}" (${thing} ${e.target_id})` : `${thing} ${e.target_id}`;
  const rule = str(d['pattern']) ? `${FIELD_NAMES[str(d['field'])] ?? str(d['field'])} matches "${str(d['pattern'])}"` : `rule #${e.target_id}`;
  switch (e.action) {
    case 'user.ban':
      return `Banned ${who}`;
    case 'user.unban':
      return `Unbanned ${who}`;
    case 'user.block_submissions':
      return `Blocked submissions from ${who}`;
    case 'user.unblock_submissions':
      return `Allowed submissions from ${who} again`;
    case 'user.delete':
      return `Deleted ${who}` + (str(d['email']) ? ` (${str(d['email'])})` : '');
    case 'item.delete':
      return `Deleted ${named}`;
    case 'item.approve':
      return `Approved the submission of ${thing} ${e.target_id}`;
    case 'item.reject':
      return `Rejected the submission of ${thing} ${e.target_id}`;
    case 'blacklist.add':
      return `Added a rule: ${rule}` + (str(d['note']) ? ` (${str(d['note'])})` : '');
    case 'blacklist.delete':
      return `Removed the rule: ${rule}`;
    case 'blacklist.purge': {
      const games = Array.isArray(d['games']) ? (d['games'] as { name?: unknown }[]).map((g) => str(g?.name)).filter(Boolean) : [];
      const removed = typeof d['removed'] === 'number' ? d['removed'] : games.length;
      return `Removed ${removed} game${removed === 1 ? '' : 's'} matching ${rule}` + (games.length ? `: ${games.join(', ')}` : '');
    }
    default:
      return `${e.action} ${e.target_type} ${e.target_id}`;
  }
}

@Component({
  selector: 'app-admin-audit',
  imports: [FormsModule, DatePipe, RouterLink],
  templateUrl: './admin-audit.html',
})
export class AdminAuditComponent implements OnInit {
  private admin = inject(AdminService);

  protected entries = signal<AuditEntry[] | null>(null);
  protected error = signal<string | null>(null);
  protected loadingMore = signal(false);
  /** False once a page comes back short: there is nothing older. */
  protected hasMore = signal(false);

  protected actions = Object.keys(ACTION_LABELS) as AuditAction[];
  protected labels = ACTION_LABELS;
  protected action: AuditAction | '' = '';
  protected describe = describeAudit;

  /** An action this page doesn't know yet (newer server) shows as its raw name. */
  protected label(action: string): string {
    return (ACTION_LABELS as Record<string, string>)[action] ?? action;
  }

  ngOnInit() {
    this.reload();
  }

  protected reload() {
    this.entries.set(null);
    this.error.set(null);
    this.fetch();
  }

  protected loadMore() {
    const list = this.entries();
    if (!list?.length) return;
    this.loadingMore.set(true);
    this.fetch(list[list.length - 1].audit_id);
  }

  /** A link to what the entry is about, where there is a page for it. */
  protected targetLink(e: AuditEntry): string | null {
    if (e.target_type === 'game' && e.action !== 'item.delete') return `/games/${e.target_id}`;
    if (e.target_type === 'bundle' && e.action !== 'item.delete') return `/bundles/${e.target_id}`;
    return null;
  }

  private fetch(before?: number) {
    this.admin.getAuditLog({ action: this.action, before, limit: PAGE_SIZE }).subscribe({
      next: (page) => {
        this.entries.set([...(before ? (this.entries() ?? []) : []), ...page]);
        this.hasMore.set(page.length === PAGE_SIZE);
        this.loadingMore.set(false);
      },
      error: (err) => {
        this.error.set(apiErrorMessage(err, "Couldn't load the audit log."));
        if (!before) this.entries.set([]);
        this.loadingMore.set(false);
      },
    });
  }
}
