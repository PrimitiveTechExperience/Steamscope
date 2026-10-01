import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';

import { AdminService } from '../../../services/admin';
import { BlacklistField, BlacklistMatch, BlacklistRule } from '../../../models/admin';
import { apiErrorMessage } from '../../../api';

const FIELD_LABELS: Record<BlacklistField, string> = {
  app_id: 'App ID',
  name: 'Game name',
  developer: 'Developer',
  publisher: 'Publisher',
};

const FIELD_HINTS: Record<BlacklistField, string> = {
  app_id: 'e.g. 730',
  name: 'regex, e.g. ^hentai|\\bnsfw\\b',
  developer: 'regex, e.g. ^shady games( ltd)?$',
  publisher: 'regex, e.g. ^some publisher',
};

@Component({
  selector: 'app-admin-blacklist',
  imports: [FormsModule, DatePipe],
  templateUrl: './admin-blacklist.html',
})
export class AdminBlacklistComponent implements OnInit {
  private admin = inject(AdminService);

  protected rules = signal<BlacklistRule[] | null>(null);
  protected error = signal<string | null>(null);
  protected notice = signal<string | null>(null);
  protected matches = signal<Record<number, BlacklistMatch[]>>({});

  protected fields = Object.keys(FIELD_LABELS) as BlacklistField[];
  protected fieldLabels = FIELD_LABELS;
  protected newField: BlacklistField = 'developer';
  protected newPattern = '';
  protected newNote = '';

  protected hint(): string {
    return FIELD_HINTS[this.newField];
  }

  ngOnInit() {
    this.load();
  }

  protected add() {
    this.error.set(null);
    this.notice.set(null);
    this.admin.addBlacklistRule({ field: this.newField, pattern: this.newPattern.trim(), note: this.newNote.trim() }).subscribe({
      next: (rule) => {
        this.newPattern = '';
        this.newNote = '';
        this.load();
        this.checkMatches(rule);
      },
      error: (err) => this.error.set(apiErrorMessage(err, "Couldn't add that rule.")),
    });
  }

  protected remove(rule: BlacklistRule) {
    if (!confirm('Remove this rule? Games it blocked can be submitted again.')) return;
    this.admin.deleteBlacklistRule(rule.rule_id).subscribe({
      next: () => this.load(),
      error: (err) => this.error.set(apiErrorMessage(err, "Couldn't remove that rule.")),
    });
  }

  /** Previews which games already on the site this rule would remove. */
  protected checkMatches(rule: BlacklistRule) {
    this.admin.getBlacklistMatches(rule.rule_id).subscribe({
      next: (list) => this.matches.update((m) => ({ ...m, [rule.rule_id]: list })),
      error: (err) => this.error.set(apiErrorMessage(err, "Couldn't check that rule.")),
    });
  }

  protected purge(rule: BlacklistRule) {
    const list = this.matches()[rule.rule_id] ?? [];
    if (!confirm(`Remove ${list.length} game(s) from the site? Their stored data is deleted.`)) return;
    this.admin.purgeBlacklistMatches(rule.rule_id).subscribe({
      next: (res) => {
        this.matches.update((m) => ({ ...m, [rule.rule_id]: [] }));
        this.notice.set(`Removed ${res.removed} game(s).`);
      },
      error: (err) => this.error.set(apiErrorMessage(err, "Couldn't remove those games.")),
    });
  }

  private load() {
    this.admin.getBlacklist().subscribe({
      next: (rules) => this.rules.set(rules),
      error: (err) => this.error.set(apiErrorMessage(err, "Couldn't load the blacklist.")),
    });
  }
}
