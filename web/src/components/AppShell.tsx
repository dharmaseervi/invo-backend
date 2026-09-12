"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { getToken } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Icon, type IconName } from "@/components/icons";
import { Button, Card, Spinner } from "@/components/ui";
import { ThemeToggle } from "@/components/ThemeToggle";

/**
 * Grouped because ten flat links is a list to read, not a menu to navigate: the
 * headings let someone find "credit notes" by knowing it is a sales document rather
 * than by scanning every item.
 */
const NAV: { group: string; items: { href: string; label: string; icon: IconName }[] }[] = [
  {
    group: "Overview",
    items: [{ href: "/dashboard", label: "Dashboard", icon: "dashboard" }],
  },
  {
    group: "Sales",
    items: [
      { href: "/invoices", label: "Invoices", icon: "invoice" },
      { href: "/estimates", label: "Estimates", icon: "estimate" },
      { href: "/credit-notes", label: "Credit notes", icon: "credit" },
    ],
  },
  {
    group: "Money",
    items: [
      { href: "/payments", label: "Payments", icon: "payment" },
      { href: "/expenses", label: "Expenses", icon: "expense" },
      { href: "/ledger", label: "Ledger", icon: "ledger" },
    ],
  },
  {
    group: "Records",
    items: [
      { href: "/clients", label: "Clients", icon: "client" },
      { href: "/items", label: "Items", icon: "item" },
    ],
  },
  {
    group: "Insight",
    items: [
      { href: "/reports", label: "Reports", icon: "report" },
      { href: "/settings", label: "Settings", icon: "settings" },
    ],
  },
];

export function AppShell({
  title,
  description,
  actions,
  children,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const { loading, company, companies, selectCompany, signOut } = useAuth();
  const [navOpen, setNavOpen] = useState(false);

  useEffect(() => {
    if (!loading && !getToken()) router.replace("/login");
  }, [loading, router]);

  if (loading) {
    return (
      <div className="grid min-h-screen place-items-center text-muted">
        <Spinner />
      </div>
    );
  }

  const sidebar = (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2.5 px-4 py-4">
        <span className="grid h-7 w-7 place-items-center rounded-[var(--radius-base)] bg-solid text-[13px] font-medium text-solid-fg">
          ₹
        </span>
        <span className="text-heading-16">Invo Billing</span>
      </div>

      <nav className="scroll-slim flex-1 overflow-y-auto px-2.5 pb-4">
        {NAV.map((section) => (
          <div key={section.group} className="mb-4">
            <p className="px-2.5 pb-1.5 text-label-12 font-medium text-muted">
              {section.group}
            </p>
            <ul className="space-y-0.5">
              {section.items.map((tab) => {
                const active = pathname?.startsWith(tab.href);
                return (
                  <li key={tab.href}>
                    <Link
                      href={tab.href}
                      // Closed on tap rather than on a route change: the drawer should
                      // not stay over the page it just navigated to.
                      onClick={() => setNavOpen(false)}
                      aria-current={active ? "page" : undefined}
                      className={`flex items-center gap-2.5 rounded-[var(--radius-base)] px-2.5 py-1.5 text-label-14 transition ${
                        active
                          ? "bg-subtle font-medium text-ink"
                          : "text-muted hover:bg-subtle hover:text-ink"
                      }`}
                    >
                      <Icon name={tab.icon} className="h-4 w-4 shrink-0" />
                      {tab.label}
                    </Link>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>

      <div className="border-t border-line p-2.5">
        <ThemeToggle />
        {companies.length > 1 ? (
          <div className="relative mb-1.5">
            <select
              aria-label="Company"
              value={company?.id ?? ""}
              onChange={(e) => selectCompany(Number(e.target.value))}
              className="w-full appearance-none rounded-[var(--radius-base)] border border-line bg-surface px-2.5 py-1.5 pr-8 text-label-13 outline-none focus:border-ink"
            >
              {companies.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
            <Icon
              name="chevron-down"
              className="pointer-events-none absolute right-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted"
            />
          </div>
        ) : (
          company && (
            <p className="truncate px-2.5 pb-1.5 text-label-13 font-medium">{company.name}</p>
          )
        )}
        <button
          type="button"
          onClick={() => void signOut()}
          className="flex w-full items-center gap-2.5 rounded-[var(--radius-base)] px-2.5 py-1.5 text-label-14 text-muted transition hover:bg-subtle hover:text-ink"
        >
          <Icon name="logout" className="h-4 w-4" />
          Sign out
        </button>
      </div>
    </div>
  );

  return (
    <div className="min-h-screen lg:flex">
      {/* Desktop rail */}
      <aside className="sticky top-0 hidden h-screen w-[15rem] shrink-0 border-r border-line bg-surface lg:block">
        {sidebar}
      </aside>

      {/* Mobile drawer */}
      {navOpen && (
        <div
          className="animate-fade-in fixed inset-0 z-40 bg-ink/40 lg:hidden"
          onMouseDown={() => setNavOpen(false)}
        >
          <div
            className="animate-slide-in h-full w-64 border-r border-line bg-surface"
            onMouseDown={(e) => e.stopPropagation()}
          >
            {sidebar}
          </div>
        </div>
      )}

      <div className="min-w-0 flex-1">
        <header className="sticky top-0 z-30 border-b border-line bg-canvas/80 backdrop-blur-md">
          <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-3 px-4 py-3.5 sm:px-6">
            <button
              type="button"
              onClick={() => setNavOpen(true)}
              aria-label="Open menu"
              className="rounded-[var(--radius-base)] p-1.5 text-muted hover:bg-subtle hover:text-ink lg:hidden"
            >
              <Icon name="menu" className="h-5 w-5" />
            </button>
            <div className="min-w-0 flex-1">
              <h1 className="text-heading-20 truncate">{title}</h1>
              {description && (
                <p className="mt-0.5 hidden truncate text-copy-13 text-muted sm:block">
                  {description}
                </p>
              )}
            </div>
            {actions && <div className="flex items-center gap-2">{actions}</div>}
          </div>
        </header>

        <main className="mx-auto max-w-6xl px-4 py-6 sm:px-6">{children}</main>
      </div>
    </div>
  );
}

/**
 * Shown instead of a screen when the account has no company. Every record belongs to
 * one, so there is nothing to list and nothing that can be created until it exists.
 */
export function NoCompany() {
  return (
    <Card className="px-6 py-16 text-center">
      <div className="mx-auto mb-4 grid h-10 w-10 place-items-center rounded-[var(--radius-base)] border border-line bg-subtle text-muted">
        <Icon name="settings" className="h-4 w-4" />
      </div>
      <p className="text-heading-16">No company yet</p>
      <p className="mx-auto mt-1.5 max-w-sm text-copy-14 text-muted">
        Invoices, clients and items all belong to a company. Create yours to get started
        — its name, GSTIN and state are what every invoice prints as the seller.
      </p>
      <div className="mt-5 flex justify-center">
        <Link href="/settings">
          <Button variant="primary" icon="settings">
            Set up company
          </Button>
        </Link>
      </div>
    </Card>
  );
}
