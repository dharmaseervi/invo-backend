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
import { Badge, Button, Card, EmptyState, Select, Spinner } from "@/components/ui";

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
      <AppShell title="Dashboard">{authLoading ? <Spinner /> : <NoCompany />}</AppShell>
    );
  }

  return (
    <AppShell
      title="Dashboard"
      actions={
        <div className="w-36">
          <Select
            label="Period"
            value={period}
            onChange={(e) => setPeriod(e.target.value as Period)}
            className="!mb-0"
          >
            <option value="week">This week</option>
            <option value="month">This month</option>
            <option value="year">This year</option>
          </Select>
        </div>
      }
    >
      {loading && !data ? (
        <div className="grid place-items-center py-24">
          <Spinner />
        </div>
      ) : error || !data ? (
        <Card>
          <EmptyState
            title="Couldn't load the dashboard"
            message={error}
            action={<Button onClick={() => void load()}>Try again</Button>}
          />
        </Card>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Card className="p-4">
              <p className="text-xs text-muted">Revenue</p>
              <p className="tabular mt-1 text-lg font-semibold">
                {formatMoney(data.revenue.total)}
              </p>
              {data.revenue.change_percent !== 0 && (
                <p
                  className={`mt-1 text-xs ${
                    data.revenue.change_percent > 0 ? "text-success" : "text-danger"
                  }`}
                >
                  {data.revenue.change_percent > 0 ? "▲" : "▼"}{" "}
                  {Math.abs(data.revenue.change_percent).toFixed(1)}% on the previous period
                </p>
              )}
            </Card>
            <Card className="p-4">
              <p className="text-xs text-muted">Invoices</p>
              <p className="tabular mt-1 text-lg font-semibold">{data.counts.invoices}</p>
            </Card>
            <Card className="p-4">
              <p className="text-xs text-muted">Clients</p>
              <p className="tabular mt-1 text-lg font-semibold">{data.counts.clients}</p>
            </Card>
            <Card className="p-4">
              <p className="text-xs text-muted">Items</p>
              <p className="tabular mt-1 text-lg font-semibold">{data.counts.items}</p>
            </Card>
          </div>

          {data.revenue.trend.length > 0 && <Trend trend={data.revenue.trend} />}

          <Card className="mt-5">
            <div className="flex items-center justify-between border-b border-line px-5 py-3">
              <p className="text-sm font-medium">Recent invoices</p>
              <Link href="/invoices" className="text-sm text-accent hover:underline">
                View all
              </Link>
            </div>
            {data.recent_invoices.length === 0 ? (
              <p className="py-10 text-center text-sm text-muted">
                No invoices in this period.
              </p>
            ) : (
              <ul className="divide-y divide-line">
                {data.recent_invoices.map((inv) => (
                  <li key={inv.id} className="flex items-center gap-4 px-5 py-3">
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
                            ? "muted"
                            : "warning"
                      }
                    >
                      {inv.status}
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
  return (
    <Card className="mt-5 p-5">
      <p className="mb-4 text-sm font-medium">Last 7 days</p>
      <div className="flex h-32 items-end gap-2">
        {trend.map((d) => (
          <div key={d.date} className="flex flex-1 flex-col items-center gap-1.5">
            <div
              className="w-full rounded-t bg-accent/80"
              style={{ height: `${Math.max((d.total / max) * 100, 2)}%` }}
              title={`${d.date}: ${formatMoney(d.total)}`}
            />
            <span className="text-[10px] text-muted">{d.date.slice(8)}</span>
          </div>
        ))}
      </div>
    </Card>
  );
}
