"use client";

import Link from "next/link";
import { useAuth } from "@/lib/auth";
import { AppShell, NoCompany } from "@/components/AppShell";
import { Card } from "@/components/ui";

const SECTIONS = [
  {
    href: "/clients",
    title: "Clients",
    blurb: "The people and businesses you invoice.",
  },
  {
    href: "/items",
    title: "Items",
    blurb: "Your catalogue, pricing and stock levels.",
  },
];

export default function DashboardPage() {
  const { company } = useAuth();

  return (
    <AppShell title="Dashboard">
      {!company ? (
        <NoCompany />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {SECTIONS.map((s) => (
            <Link key={s.href} href={s.href} className="block">
              <Card className="p-5 transition hover:border-accent">
                <p className="text-sm font-medium">{s.title}</p>
                <p className="mt-1 text-sm text-muted">{s.blurb}</p>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </AppShell>
  );
}
