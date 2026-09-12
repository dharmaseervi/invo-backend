import type { Metadata } from "next";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import "./globals.css";
import { AuthProvider } from "@/lib/auth";
import { ToastProvider } from "@/components/ui";
import { THEME_BOOTSTRAP } from "@/components/ThemeToggle";

// Geist Sans and Geist Mono, the typefaces the design system is built on. The font
// files ship inside the `geist` package and are served from our own origin, so unlike
// next/font/google this adds no network dependency to the Docker build.
export const metadata: Metadata = {
  title: "Invo Billing",
  description: "GST invoicing for Indian businesses",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="en"
      className={`h-full antialiased ${GeistSans.variable} ${GeistMono.variable}`}
      suppressHydrationWarning
    >
      <head>
        {/* Applies the stored theme before first paint. Without it a user who chose
            light on a dark machine sees a dark flash on every page load. */}
        <script dangerouslySetInnerHTML={{ __html: THEME_BOOTSTRAP }} />
      </head>
      <body className="h-full">
        <AuthProvider>
          <ToastProvider>{children}</ToastProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
