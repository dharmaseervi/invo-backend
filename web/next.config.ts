import type { NextConfig } from "next";

// Production is served by Go on the same origin. Refuse accidental bearer-token
// storage builds rather than relying on a deployment convention.
if (process.env.NODE_ENV === "production" && process.env.NEXT_PUBLIC_API_ORIGIN) {
  throw new Error("Production builds must leave NEXT_PUBLIC_API_ORIGIN unset; authentication uses same-origin HttpOnly cookies.");
}

const nextConfig: NextConfig = {
  // Static export: the Go server hosts the result, so there is no Node runtime in
  // production. Nothing here needs SSR — every screen is behind a login and fetches
  // per-user data with an HttpOnly cookie, which has to happen in the browser anyway.
  output: "export",

  // Served from https://invobilling.com/app, so every asset and route is prefixed.
  basePath: "/app",
  assetPrefix: "/app",

  // Export writes index.html inside a directory per route, which is what lets the Go
  // server resolve /app/invoices without a rewrite rule for each one.
  trailingSlash: true,

  // The optimiser needs a running server; there is none.
  images: { unoptimized: true },
};

export default nextConfig;
