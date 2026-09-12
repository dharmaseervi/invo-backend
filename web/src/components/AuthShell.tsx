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
        className="pointer-events-none absolute inset-0 opacity-[0.55]"
        style={{
          background:
            "radial-gradient(60rem 30rem at 50% -8rem, var(--color-accent-soft), transparent 70%)",
        }}
      />

      <div className="relative w-full max-w-[400px]">
        <div className="mb-7 text-center">
          <Link
            href="/"
            className="inline-flex items-center gap-2.5 rounded-lg px-1 py-0.5"
          >
            <span className="grid h-9 w-9 place-items-center rounded-[10px] bg-accent text-base font-semibold text-accent-fg shadow-sm">
              ₹
            </span>
            <span className="text-[17px] font-semibold tracking-tight">Invo Billing</span>
          </Link>
          <h1 className="mt-5 text-[22px] font-semibold tracking-tight">{title}</h1>
          {subtitle && (
            <p className="mx-auto mt-1.5 max-w-[19rem] text-sm leading-relaxed text-muted">
              {subtitle}
            </p>
          )}
        </div>

        <Card className="p-6 shadow-md">{children}</Card>

        {footer && <div className="mt-5 text-center text-sm">{footer}</div>}
      </div>
    </div>
  );
}
