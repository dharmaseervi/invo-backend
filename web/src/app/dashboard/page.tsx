"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  dashboard as dashboardApi,
  formatDate,
  formatMoney,
  type Dashboard,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { AppShell, NoCompany } from "@/components/AppShell";
import { Icon } from "@/components/icons";
import {
  Badge,
  Button,
  Card,
  CardHead,
  EmptyState,
  Skeleton,
  Stat,
} from "@/components/ui";

type Period = "week" | "month" | "year";

export default function DashboardPage() {
  const { company, loading: authLoading } = useAuth();
  const [period, setPeriod] = useState<Period>("month");
  const [data, setData] = useState<Dashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const companyId = company?.id ?? null;

  const load = useCallback(async () => {
    if (!companyId) return;
    setLoading(true);
    setError("");
    try {
      setData(await dashboardApi.get(companyId, period));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load the dashboard");
    } finally {
      setLoading(false);
    }
  }, [companyId, period]);

  useEffect(() => {
    if (!companyId) return;
    void (async () => {
      await load();
    })();
  }, [companyId, load]);

  if (authLoading || !company) {
    return (
      <AppShell title="Dashboard">{authLoading ? null : <NoCompany />}</AppShell>
    );
  }

  return (
    <AppShell
      title="Dashboard"
      description="How the business is doing this period"
      actions={
        <div className="relative">
          <select
            aria-label="Period"
            value={period}
            onChange={(e) => setPeriod(e.target.value as Period)}
            className="h-9.5 appearance-none rounded-lg border border-line bg-surface pl-3 pr-8 text-sm shadow-xs outline-none focus:border-accent"
          >
            <option value="week">This week</option>
            <option value="month">This month</option>
            <option value="year">This year</option>
          </select>
          <Icon
            name="chevron-down"
            className="pointer-events-none absolute right-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted"
          />
        </div>
      }
    >
      {loading && !data ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Card key={i} className="p-4">
              <Skeleton className="h-3 w-20" />
              <Skeleton className="mt-3 h-6 w-28" />
            </Card>
          ))}
        </div>
      ) : error || !data ? (
        <Card>
          <EmptyState
            icon="alert"
            title="Couldn't load the dashboard"
            message={error}
            action={<Button onClick={() => void load()}>Try again</Button>}
          />
        </Card>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Stat
              label="Revenue"
              value={formatMoney(data.revenue.total)}
              icon="payment"
              sub={
                data.revenue.change_percent !== 0 && (
                  <span
                    className={
                      data.revenue.change_percent > 0 ? "text-success" : "text-danger"
                    }
                  >
                    {data.revenue.change_percent > 0 ? "↑" : "↓"}{" "}
                    {Math.abs(data.revenue.change_percent).toFixed(1)}% on the previous
                    period
                  </span>
                )
              }
            />
            <Stat label="Invoices" value={String(data.counts.invoices)} icon="invoice" />
            <Stat label="Clients" value={String(data.counts.clients)} icon="client" />
            <Stat label="Items" value={String(data.counts.items)} icon="item" />
          </div>

          {data.revenue.trend.length > 0 && <Trend trend={data.revenue.trend} />}

          <Card className="mt-5">
            <CardHead
              title="Recent invoices"
              action={
                <Link
                  href="/invoices"
                  className="text-[13px] font-medium text-accent hover:underline"
                >
                  View all
                </Link>
              }
            />
            {data.recent_invoices.length === 0 ? (
              <EmptyState
                icon="invoice"
                title="Nothing yet this period"
                message="Invoices you raise will show up here."
              />
            ) : (
              <ul className="divide-y divide-line">
                {data.recent_invoices.map((inv) => (
                  <li
                    key={inv.id}
                    className="flex items-center gap-4 px-5 py-3 transition hover:bg-subtle/60"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{inv.invoice_number}</p>
                      <p className="truncate text-xs text-muted">
                        {inv.client_name} · {formatDate(inv.created_at)}
                      </p>
                    </div>
                    <Badge
                      tone={
                        inv.status === "paid"
                          ? "success"
                          : inv.status === "cancelled"
                            ? "neutral"
                            : inv.status === "draft"
                              ? "neutral"
                              : "info"
                      }
                    >
                      {/* The API returns the raw column value; capitalise it rather
                          than showing "issued" mid-sentence in a badge. */}
                      {inv.status.charAt(0).toUpperCase() + inv.status.slice(1)}
                    </Badge>
                    <p className="tabular w-28 text-right text-sm font-medium">
                      {formatMoney(inv.total)}
                    </p>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </>
      )}
    </AppShell>
  );
}

/**
 * Last seven days of revenue as bars, scaled to the largest day. Plain divs rather than
 * a chart library: it is seven values, and a dependency for that is not worth the
 * bundle or the CDN.
 */
function Trend({ trend }: { trend: { date: string; total: number }[] }) {
  const max = Math.max(...trend.map((d) => d.total), 1);
  const best = trend.reduce((a, b) => (b.total > a.total ? b : a), trend[0]);
  return (
    <Card className="mt-5">
      <CardHead
        title="Last 7 days"
        action={
          <span className="text-xs text-muted">Best day {formatMoney(best?.total ?? 0)}</span>
        }
      />
      <div className="px-5 py-5">
        <div className="flex h-36 items-end gap-1.5 border-b border-line pb-px">
          {trend.map((d) => (
            <div
              key={d.date}
              className="group flex h-full flex-1 flex-col justify-end gap-1.5"
              title={`${d.date}: ${formatMoney(d.total)}`}
            >
              <span className="tabular text-center text-[10px] text-muted opacity-0 transition group-hover:opacity-100">
                {d.total > 0 ? formatMoney(d.total) : ""}
              </span>
              <div
                className="w-full rounded-t-md bg-accent/25 transition group-hover:bg-accent/45"
                style={{ height: `${Math.max((d.total / max) * 100, 2)}%` }}
              >
                <div className="h-1 w-full rounded-t-md bg-accent" />
              </div>
            </div>
          ))}
        </div>
        <div className="mt-1.5 flex gap-1.5">
          {trend.map((d) => (
            <span key={d.date} className="tabular flex-1 text-center text-[10px] text-muted">
              {d.date.slice(8)}
            </span>
          ))}
        </div>
      </div>
    </Card>
  );
}
