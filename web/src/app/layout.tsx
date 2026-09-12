import type { Metadata } from "next";
import "./globals.css";
import { AuthProvider } from "@/lib/auth";
import { ToastProvider } from "@/components/ui";
import { THEME_BOOTSTRAP } from "@/components/ThemeToggle";

// System fonts on purpose: next/font/google fetches at build time, which would make
// the Docker build depend on network access to Google. Not worth the fragility.
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
    <html lang="en" className="h-full antialiased" suppressHydrationWarning>
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
