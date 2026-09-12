"use client";

import { useEffect } from "react";

/**
 * Browser error reporting, off unless NEXT_PUBLIC_SENTRY_DSN is set at build time.
 *
 * The SDK is imported dynamically so it is only downloaded when a DSN is configured —
 * an unconfigured build ships none of it. A browser DSN is public by design; it can
 * only submit events, which is why it is safe to bake into a static export.
 */
export function ErrorReporting() {
  useEffect(() => {
    const dsn = process.env.NEXT_PUBLIC_SENTRY_DSN;
    if (!dsn) return;

    void (async () => {
      try {
        const Sentry = await import("@sentry/browser");
        Sentry.init({
          dsn,
          environment: process.env.NEXT_PUBLIC_SENTRY_ENV ?? "production",
          release: process.env.NEXT_PUBLIC_RELEASE,

          // This is a billing app: an error's surroundings include client names,
          // invoice amounts and GSTINs, and none of that belongs in a third-party
          // tracker. Nothing personal is attached on purpose.
          sendDefaultPii: false,
          // Errors only. Tracing every navigation would exhaust a free tier in days
          // and tells us nothing the server timings do not.
          tracesSampleRate: 0,

          // Only our own code. A browser extension throwing inside the page is not
          // a bug in this app, and those dominate unfiltered browser reporting.
          allowUrls: [window.location.origin],

          beforeBreadcrumb(crumb) {
            // Query strings carry what the user typed — a client's name in a search
            // box, a reset code in a verification link. The path is enough to know
            // which request failed.
            if (crumb.data?.url && typeof crumb.data.url === "string") {
              crumb.data.url = crumb.data.url.split("?")[0];
            }
            return crumb;
          },

          beforeSend(event) {
            // The URL of the page is on every event; strip its query the same way.
            if (event.request?.url) {
              event.request.url = event.request.url.split("?")[0];
            }
            if (event.request?.headers) delete event.request.headers;
            return event;
          },
        });
      } catch {
        // Reporting failing to load must never break the page it is reporting on.
      }
    })();
  }, []);

  return null;
}
