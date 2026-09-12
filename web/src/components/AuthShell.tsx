import Link from "next/link";
import { Card } from "./ui";

/**
 * Shared frame for every signed-out screen, so login, signup and reset match.
 *
 * The panel sits on a soft radial wash rather than a flat canvas — the one place in
 * the app with no data to look at, so the surface itself does the work of making it
 * feel finished.
 */
export function AuthShell({
  title,
  subtitle,
  children,
  footer,
}: {
  title: string;
  subtitle?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
}) {
  return (
    <div className="relative grid min-h-full place-items-center overflow-hidden p-5">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0"
        style={{
          // A faint grid rather than a colour wash: the ruled page is the part of the
          // Vercel aesthetic that carries, and it stays quiet behind the panel.
          backgroundImage:
            "linear-gradient(var(--color-line) 1px, transparent 1px), linear-gradient(90deg, var(--color-line) 1px, transparent 1px)",
          backgroundSize: "64px 64px",
          maskImage: "radial-gradient(40rem 28rem at 50% 30%, #000, transparent 75%)",
          opacity: 0.5,
        }}
      />

      <div className="relative w-full max-w-[400px]">
        <div className="mb-7 text-center">
          <Link
            href="/"
            className="inline-flex items-center gap-2.5 rounded-lg px-1 py-0.5"
          >
            <span className="grid h-8 w-8 place-items-center rounded-[var(--radius-base)] bg-accent text-label-14 font-medium text-accent-fg">
              ₹
            </span>
            <span className="text-heading-16">Invo Billing</span>
          </Link>
          <h1 className="text-heading-24 mt-5">{title}</h1>
          {subtitle && (
            <p className="text-copy-14 mx-auto mt-2 max-w-[19rem] text-muted">{subtitle}</p>
          )}
        </div>

        <Card className="p-6">{children}</Card>

        {footer && <div className="mt-5 text-center text-sm">{footer}</div>}
      </div>
    </div>
  );
}
