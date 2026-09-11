import Link from "next/link";
import { Card } from "./ui";

/** Shared frame for every signed-out screen, so login, signup and reset match. */
export function AuthShell({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid min-h-full place-items-center p-5">
      <div className="w-full max-w-[400px]">
        <div className="mb-6 text-center">
          <Link href="/" className="inline-flex items-center gap-2.5">
            <span className="grid h-9 w-9 place-items-center rounded-[10px] bg-accent text-base font-bold text-accent-fg">
              ₹
            </span>
            <span className="text-[17px] font-semibold tracking-tight">
              Invo Billing
            </span>
          </Link>
          <h1 className="mt-4 text-xl font-semibold tracking-tight">{title}</h1>
          {subtitle && <p className="mt-1 text-sm text-muted">{subtitle}</p>}
        </div>
        <Card className="p-6">{children}</Card>
      </div>
    </div>
  );
}
