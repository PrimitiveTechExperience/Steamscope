import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';

import { AccountService } from '../../services/account';
import { Submission } from '../../models/user';
import { apiErrorMessage } from '../../api';

const STORE_URL = /^https?:\/\/store\.steampowered\.com\/app\/\d+/;
const POLL_MS = 4000;

@Component({
  selector: 'app-submit',
  imports: [FormsModule, RouterLink, DatePipe],
  templateUrl: './submit.html',
})
export class SubmitComponent implements OnInit {
  private account = inject(AccountService);
  private destroyRef = inject(DestroyRef);
  private pollTimer: ReturnType<typeof setTimeout> | null = null;

  protected url = '';
  protected submitting = signal(false);
  protected message = signal<{ kind: 'ok' | 'error'; text: string } | null>(null);
  protected submissions = signal<Submission[] | null>(null);

  constructor() {
    this.destroyRef.onDestroy(() => this.stopPolling());
  }

  ngOnInit() {
    this.loadSubmissions();
  }

  protected submit() {
    const url = this.url.trim();
    if (!STORE_URL.test(url)) {
      this.message.set({ kind: 'error', text: 'Paste a Steam store link, like https://store.steampowered.com/app/730/' });
      return;
    }
    this.submitting.set(true);
    this.message.set(null);
    this.account.submitGame(url).subscribe({
      next: (res) => {
        this.submitting.set(false);
        this.url = '';
        this.message.set(
          res.status === 'tracked'
            ? { kind: 'ok', text: 'Good news - that game is already being tracked.' }
            : { kind: 'ok', text: "Thanks! We're fetching that game now. You'll get a notification when it's added." }
        );
        this.loadSubmissions();
      },
      error: (err) => {
        this.submitting.set(false);
        this.message.set({ kind: 'error', text: apiErrorMessage(err, 'Submission failed.') });
      },
    });
  }

  private loadSubmissions() {
    this.stopPolling();
    this.account.getSubmissions().subscribe({
      next: (list) => {
        this.submissions.set(list);
        // Keep refreshing while anything is still being scraped.
        if (list.some((s) => s.status === 'pending')) {
          this.pollTimer = setTimeout(() => this.loadSubmissions(), POLL_MS);
        }
      },
      error: () => this.submissions.set([]),
    });
  }

  private stopPolling() {
    if (this.pollTimer) {
      clearTimeout(this.pollTimer);
      this.pollTimer = null;
    }
  }
}
