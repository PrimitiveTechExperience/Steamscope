import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { DatePipe } from '@angular/common';

import { AccountService } from '../../services/account';
import { Submission, SubmissionStatus } from '../../models/user';
import { apiErrorMessage } from '../../api';

const STORE_URL = /^https?:\/\/store\.steampowered\.com\/(app|bundle)\/\d+/;
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
      this.message.set({ kind: 'error', text: 'Paste a Steam game or bundle link, like https://store.steampowered.com/app/730/' });
      return;
    }
    this.submitting.set(true);
    this.message.set(null);
    this.account.submitGame(url).subscribe({
      next: (res) => {
        this.submitting.set(false);
        this.url = '';
        this.message.set({ kind: 'ok', text: this.resultText(res.status) });
        this.loadSubmissions();
      },
      error: (err) => {
        this.submitting.set(false);
        this.message.set({ kind: 'error', text: apiErrorMessage(err, 'Submission failed.') });
      },
    });
  }

  protected statusLabel(status: SubmissionStatus): string {
    switch (status) {
      case 'awaiting_approval':
        return 'Awaiting approval';
      case 'pending':
        return 'Fetching...';
      case 'rejected':
        return 'Not approved';
      default:
        return status;
    }
  }

  private resultText(status: SubmissionStatus): string {
    switch (status) {
      case 'tracked':
        return 'Good news - that is already being tracked.';
      case 'awaiting_approval':
        return "Thanks! An admin will review your submission, and you'll get a notification once it's decided.";
      case 'rejected':
        return "That one was reviewed before and wasn't approved.";
      default:
        return "Thanks! We're fetching that now. You'll get a notification when it's added.";
    }
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
