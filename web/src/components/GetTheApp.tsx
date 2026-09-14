"use client";

import { useEffect, useState } from "react";

/** App Store id for Invo Billing, from App Store Connect → App Information. */
const APP_STORE_ID = "6811639667";
const APP_STORE_URL = `https://apps.apple.com/app/id${APP_STORE_ID}`;
const DISMISSED_KEY = "invo_app_banner_dismissed";

/**
 * A bar offering the iPhone app to people using the web version on iOS.
 *
 * Safari has a better answer than anything we can write: the `apple-itunes-app` meta
 * tag in the layout renders Apple's own Smart App Banner, which knows whether the app
 * is already installed and says "Open" rather than "Get". The web cannot determine
 * that — nothing in the browser can see the installed apps — so wherever Apple's
 * banner appears, we leave it alone.
 *
 * This covers the rest: Chrome, Firefox, Edge and in-app browsers on iOS, none of
 * which render the Smart App Banner. There we show our own bar and accept that it
 * cannot know about an existing install; tapping through lands on the App Store page,
 * which says "Open" if they already have it.
 */
export function GetTheApp() {
  const [show, setShow] = useState(false);

  // After mount, never in a state initialiser: these pages are prerendered at build
  // time where there is no navigator and no localStorage, and guessing would be a
  // hydration mismatch.
  useEffect(() => {
    const ua = navigator.userAgent;
    const isIOS =
      /iPad|iPhone|iPod/.test(ua) ||
      // iPadOS 13+ reports itself as a Mac; the touch points give it away.
      (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
    if (!isIOS) return;

    // Every iOS browser is WebKit and says "Safari" somewhere in its user agent.
    // Only the real Safari lacks one of these vendor markers, and only the real
    // Safari renders the Smart App Banner.
    const isRealSafari = !/CriOS|FxiOS|EdgiOS|OPiOS|GSA|FBAN|FBAV|Instagram|Line/i.test(ua);
    if (isRealSafari) return;

    // Standalone means it is already running from the Home Screen as a PWA, where an
    // ad for a different app is just noise.
    const standalone =
      window.matchMedia("(display-mode: standalone)").matches ||
      (window.navigator as { standalone?: boolean }).standalone === true;
    if (standalone) return;

    try {
      if (localStorage.getItem(DISMISSED_KEY) === "1") return;
    } catch {
      // Private browsing can throw on access. Showing the bar is the safe failure.
    }

    setShow(true);
  }, []);

  if (!show) return null;

  function dismiss() {
    setShow(false);
    try {
      localStorage.setItem(DISMISSED_KEY, "1");
    } catch {
      // Not worth surfacing: the bar is gone for this page view either way.
    }
  }

  return (
    <div className="flex items-center gap-3 border-b border-[var(--border)] bg-[var(--muted)] px-4 py-2.5 text-sm">
      <button
        type="button"
        onClick={dismiss}
        aria-label="Dismiss"
        className="shrink-0 rounded p-1 text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
      >
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
          <path d="M1 1l12 12M13 1L1 13" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        </svg>
      </button>

      <div className="min-w-0 flex-1">
        <p className="truncate font-medium text-[var(--foreground)]">Invo Billing for iPhone</p>
        <p className="truncate text-xs text-[var(--muted-foreground)]">
          Scan barcodes, print labels and bill offline
        </p>
      </div>

      <a
        href={APP_STORE_URL}
        target="_blank"
        rel="noopener noreferrer"
        className="shrink-0 rounded-md bg-[var(--primary)] px-3 py-1.5 text-xs font-medium text-[var(--primary-foreground)]"
      >
        Get
      </a>
    </div>
  );
}
