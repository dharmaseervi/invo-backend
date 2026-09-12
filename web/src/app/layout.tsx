import type { Metadata } from "next";
import "./globals.css";
import { AuthProvider } from "@/lib/auth";
import { ToastProvider } from "@/components/ui";

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
    <html lang="en" className="h-full antialiased">
      <body className="h-full">
        <AuthProvider>
          <ToastProvider>{children}</ToastProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
