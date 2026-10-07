import { Injectable, PLATFORM_ID, inject } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

/** The service worker that shows push notifications; it lives in public/. */
export const PUSH_WORKER_URL = '/push-sw.js';

/** Why push could not be turned on. The text is safe to show the user. */
export class PushError extends Error {}

/** A browser subscription in the shape the API stores. */
export interface PushSubscriptionData {
  endpoint: string;
  keys: { p256dh: string; auth: string };
}

/** The key the browser needs, from base64url text to bytes. */
export function urlBase64ToBytes(base64: string): Uint8Array<ArrayBuffer> {
  const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
  const raw = atob(padded.replace(/-/g, '+').replace(/_/g, '/'));
  const bytes = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
  return bytes;
}

/** Turns browser push notifications on and off for this browser. */
@Injectable({ providedIn: 'root' })
export class PushService {
  private isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  /** Whether this browser can do web push at all. */
  get supported(): boolean {
    return this.isBrowser && 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
  }

  /** Whether the user has blocked notifications for this site in the browser. */
  get blocked(): boolean {
    return this.supported && Notification.permission === 'denied';
  }

  /** Asks permission if needed, registers the worker and subscribes this browser. */
  async enable(vapidPublicKey: string): Promise<PushSubscriptionData> {
    if (!this.supported) throw new PushError("This browser doesn't support push notifications.");
    if (!vapidPublicKey) throw new PushError('Push notifications are not set up on this server.');

    const permission = Notification.permission === 'granted' ? 'granted' : await Notification.requestPermission();
    if (permission !== 'granted') {
      throw new PushError('Notifications are blocked for this site. Allow them in your browser settings, then try again.');
    }
    const registration = await navigator.serviceWorker.register(PUSH_WORKER_URL);
    await navigator.serviceWorker.ready;
    let subscription = await registration.pushManager.getSubscription();
    if (!subscription) {
      try {
        subscription = await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: urlBase64ToBytes(vapidPublicKey) });
      } catch {
        throw new PushError("Couldn't subscribe this browser to push notifications.");
      }
    }
    return toData(subscription);
  }

  /** The subscription this browser already has, if any. */
  async current(): Promise<PushSubscriptionData | null> {
    if (!this.supported) return null;
    try {
      const registration = await navigator.serviceWorker.getRegistration(PUSH_WORKER_URL);
      const subscription = await registration?.pushManager.getSubscription();
      return subscription ? toData(subscription) : null;
    } catch {
      return null;
    }
  }

  /** Unsubscribes this browser, returning the endpoint it had (so the server can forget it). */
  async disable(): Promise<string | null> {
    if (!this.supported) return null;
    try {
      const registration = await navigator.serviceWorker.getRegistration(PUSH_WORKER_URL);
      const subscription = await registration?.pushManager.getSubscription();
      if (!subscription) return null;
      const endpoint = subscription.endpoint;
      await subscription.unsubscribe();
      return endpoint;
    } catch {
      return null;
    }
  }
}

function toData(subscription: PushSubscription): PushSubscriptionData {
  const json = subscription.toJSON();
  return { endpoint: subscription.endpoint, keys: { p256dh: json.keys?.['p256dh'] ?? '', auth: json.keys?.['auth'] ?? '' } };
}
