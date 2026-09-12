"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import { getToken } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Button, Spinner } from "@/components/ui";

const NAV = [
  { href: "/dashboard", label: "Dashboard" },
  { href: "/clients", label: "Clients" },
  { href: "/items", label: "Items" },
];

/**
 * Chrome for every signed-in screen: navigation, the company being worked in, and the
 * route guard.
 *
 * The guard is presentation only — the server rejects an unauthenticated request no
 * matter what this renders. It exists so a signed-out visitor never sees an empty
 * shell flash before the redirect.
 */
export function AppShell({
  title,
  actions,
  children,
}: {
  title: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const { loading, company, companies, selectCompany, signOut } = useAuth();

  useEffect(() => {
    if (!loading && !getToken()) router.replace("/login");
  }, [loading, router]);

  if (loading) {
    return (
      <div className="grid min-h-screen place-items-center">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="min-h-screen">
      <header className="border-b border-line bg-surface">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-3 px-6 py-3">
          <span className="text-sm font-semibold tracking-tight">Invo Billing</span>

          <nav className="flex items-center gap-1">
            {NAV.map((tab) => {
              const active = pathname?.startsWith(tab.href);
              return (
                <Link
                  key={tab.href}
                  href={tab.href}
                  aria-current={active ? "page" : undefined}
                  className={`rounded-lg px-3 py-1.5 text-sm transition ${
                    active ? "bg-subtle font-medium text-ink" : "text-muted hover:bg-subtle"
                  }`}
                >
                  {tab.label}
                </Link>
              );
            })}
          </nav>

          <div className="ml-auto flex items-center gap-3">
            {/* Only worth showing when there is a choice to make. */}
            {companies.length > 1 ? (
              <select
                aria-label="Company"
                value={company?.id ?? ""}
                onChange={(e) => selectCompany(Number(e.target.value))}
                className="rounded-lg border border-line bg-surface px-2.5 py-1.5 text-sm outline-none focus:ring-2 focus:ring-accent"
              >
                {companies.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            ) : (
              company && <span className="text-sm text-muted">{company.name}</span>
            )}
            <Button onClick={() => void signOut()}>Sign out</Button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-8">
        <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          {actions}
        </div>
        {children}
      </main>
    </div>
  );
}

/**
 * Shown instead of a list when the account has no company. Every clients/items request
 * is scoped to one, so there is nothing meaningful to render — and no way to create
 * anything — until a company exists.
 */
export function NoCompany() {
  return (
    <div className="rounded-xl border border-line bg-surface px-6 py-16 text-center shadow-sm">
      <p className="text-sm font-medium">No company yet</p>
      <p className="mx-auto mt-1.5 max-w-sm text-sm text-muted">
        Clients and items belong to a company. Create one in the mobile app to get
        started — company setup is coming to the web shortly.
      </p>
    </div>
  );
}
