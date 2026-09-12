"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ApiError,
  formatDate,
  formatMoney,
  formatQty,
  reports as reportsApi,
  type AgingReport,
  type GSTReport,
  type StockReport,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { downloadCsv } from "@/lib/csv";
import { AppShell, NoCompany } from "@/components/AppShell";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  Field,
  SearchInput,
  Select,
  Spinner,
  useToast,
} from "@/components/ui";

type Tab = "stock" | "aging" | "gst";

const TABS: { id: Tab; label: string }[] = [
  { id: "stock", label: "Stock" },
  { id: "aging", label: "Receivables" },
  { id: "gst", label: "GSTR-1" },
];

export default function ReportsPage() {
  const { company, loading: authLoading } = useAuth();
  const [tab, setTab] = useState<Tab>("stock");

  if (authLoading || !company) {
    return (
      <AppShell title="Reports">{authLoading ? <Spinner /> : <NoCompany />}</AppShell>
    );
  }

  return (
    <AppShell title="Reports">
      <div className="mb-5 flex gap-1">
        {TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => setTab(t.id)}
            aria-current={tab === t.id ? "page" : undefined}
            className={`rounded-lg px-3 py-1.5 text-sm transition ${
              tab === t.id ? "bg-subtle font-medium" : "text-muted hover:bg-subtle"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "stock" && <StockTab companyId={company.id} />}
      {tab === "aging" && <AgingTab companyId={company.id} />}
      {tab === "gst" && <GSTTab companyId={company.id} />}
    </AppShell>
  );
}

function Stat({ label, value, tone }: { label: string; value: string; tone?: "danger" | "warning" }) {
  return (
    <Card className="p-4">
      <p className="text-xs text-muted">{label}</p>
      <p
        className={`tabular mt-1 text-lg font-semibold ${
          tone === "danger" ? "text-danger" : tone === "warning" ? "text-warning" : ""
        }`}
      >
        {value}
      </p>
    </Card>
  );
}

/* ---------- Stock ---------- */

function StockTab({ companyId }: { companyId: number }) {
  const toast = useToast();
  const [report, setReport] = useState<StockReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState("");
  const [status, setStatus] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setReport(await reportsApi.stock(companyId));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load the stock report");
    } finally {
      setLoading(false);
    }
  }, [companyId]);

  useEffect(() => {
    void (async () => {
      await load();
    })();
  }, [load]);

  // Filtering is done here rather than on the server: the report is a single snapshot
  // the endpoint returns whole, so narrowing it needs no further round trip.
  const rows = useMemo(() => {
    if (!report) return [];
    const term = search.trim().toLowerCase();
    return report.items.filter((i) => {
      if (status && i.status !== status) return false;
      if (category && String(i.category_id ?? "") !== category) return false;
      if (term && !`${i.name} ${i.sku}`.toLowerCase().includes(term)) return false;
      return true;
    });
  }, [report, search, category, status]);

  if (loading && !report) {
    return (
      <Card>
        <div className="grid place-items-center py-16">
          <Spinner />
        </div>
      </Card>
    );
  }
  if (error || !report) {
    return (
      <Card>
        <EmptyState
          title="Couldn't load the report"
          message={error}
          action={<Button onClick={() => void load()}>Try again</Button>}
        />
      </Card>
    );
  }

  return (
    <>
      <div className="mb-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Stock at cost" value={formatMoney(report.total_cost_value)} />
        <Stat label="Retail value" value={formatMoney(report.total_retail_value)} />
        <Stat
          label="Low stock"
          value={String(report.low_stock_count)}
          tone={report.low_stock_count > 0 ? "warning" : undefined}
        />
        <Stat
          label="Out of stock"
          value={String(report.out_of_stock_count)}
          tone={report.out_of_stock_count > 0 ? "danger" : undefined}
        />
      </div>

      <Card className="mb-5 p-5">
        <p className="mb-3 text-sm font-medium">By category</p>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[40rem] text-sm">
            <thead>
              <tr className="border-b border-line text-left text-xs text-muted">
                <th className="py-2 font-medium">Category</th>
                <th className="w-20 py-2 text-right font-medium">Items</th>
                <th className="w-20 py-2 text-right font-medium">Units</th>
                <th className="w-32 py-2 text-right font-medium">At cost</th>
                <th className="w-32 py-2 text-right font-medium">At retail</th>
                <th className="w-24 py-2 text-right font-medium">Share</th>
              </tr>
            </thead>
            <tbody>
              {report.categories.map((c) => (
                <tr key={c.category_id ?? "none"} className="border-b border-line">
                  <td className="py-2">{c.category_name}</td>
                  <td className="tabular py-2 text-right">{c.item_count}</td>
                  <td className="tabular py-2 text-right">{formatQty(c.total_units)}</td>
                  <td className="tabular py-2 text-right">{formatMoney(c.cost_value)}</td>
                  <td className="tabular py-2 text-right">{formatMoney(c.retail_value)}</td>
                  <td className="tabular py-2 text-right">{c.share_of_value.toFixed(1)}%</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      <div className="mb-3 flex flex-wrap items-end gap-3">
        <SearchInput value={search} onChange={setSearch} placeholder="Search name or SKU" />
        <div className="w-44">
          <Select
            label="Category"
            value={category}
            onChange={(e) => setCategory(e.target.value)}
            className="!mb-0"
          >
            <option value="">All categories</option>
            {report.categories.map((c) => (
              <option key={c.category_id ?? "none"} value={String(c.category_id ?? "")}>
                {c.category_name}
              </option>
            ))}
          </Select>
        </div>
        <div className="w-40">
          <Select label="Status" value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="">All</option>
            <option value="in">In stock</option>
            <option value="low">Low</option>
            <option value="out">Out of stock</option>
          </Select>
        </div>
        <Button
          className="mb-4"
          onClick={() => {
            downloadCsv(
              `stock-${report.as_of}.csv`,
              [
                "Name", "SKU", "Category", "Unit", "Quantity",
                "Cost price", "Selling price", "Stock value", "Retail value", "Status",
              ],
              rows.map((i) => [
                i.name, i.sku, i.category_name, i.unit, i.quantity,
                i.cost_price, i.price, i.stock_value, i.retail_value, i.status,
              ]),
            );
            toast.show(`Exported ${rows.length} rows`, "success");
          }}
        >
          Export CSV
        </Button>
      </div>

      <Card>
        {rows.length === 0 ? (
          <EmptyState title="Nothing matches" message="Adjust the filters above." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[44rem] text-sm">
              <thead>
                <tr className="border-b border-line text-left text-xs text-muted">
                  <th className="px-5 py-2.5 font-medium">Item</th>
                  <th className="w-24 py-2.5 text-right font-medium">Qty</th>
                  <th className="w-32 py-2.5 text-right font-medium">At cost</th>
                  <th className="w-32 py-2.5 text-right font-medium">At retail</th>
                  <th className="w-28 px-5 py-2.5 text-right font-medium">Status</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((i) => (
                  <tr key={i.id} className="border-b border-line last:border-0">
                    <td className="px-5 py-2.5">
                      <p className="font-medium">{i.name}</p>
                      <p className="text-xs text-muted">
                        {[i.sku, i.category_name].filter(Boolean).join(" · ")}
                      </p>
                    </td>
                    <td className="tabular py-2.5 text-right">
                      {formatQty(i.quantity)} {i.unit}
                    </td>
                    <td className="tabular py-2.5 text-right">{formatMoney(i.stock_value)}</td>
                    <td className="tabular py-2.5 text-right">{formatMoney(i.retail_value)}</td>
                    <td className="px-5 py-2.5 text-right">
                      {i.status === "out" ? (
                        <Badge tone="danger">Out</Badge>
                      ) : i.status === "low" ? (
                        <Badge tone="warning">Low</Badge>
                      ) : (
                        <Badge tone="success">In stock</Badge>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
      <p className="mt-3 text-xs text-muted">
        {rows.length} of {report.total_items} items · as of {formatDate(report.as_of)}
      </p>
    </>
  );
}

/* ---------- Receivables ageing ---------- */

function AgingTab({ companyId }: { companyId: number }) {
  const [report, setReport] = useState<AgingReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setReport(await reportsApi.aging(companyId));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load the report");
    } finally {
      setLoading(false);
    }
  }, [companyId]);

  useEffect(() => {
    void (async () => {
      await load();
    })();
  }, [load]);

  if (loading && !report) {
    return (
      <Card>
        <div className="grid place-items-center py-16">
          <Spinner />
        </div>
      </Card>
    );
  }
  if (error || !report) {
    return (
      <Card>
        <EmptyState
          title="Couldn't load the report"
          message={error}
          action={<Button onClick={() => void load()}>Try again</Button>}
        />
      </Card>
    );
  }

  const cols = [
    ["Not due", "current"],
    ["1–30 days", "days_1_30"],
    ["31–60 days", "days_31_60"],
    ["61–90 days", "days_61_90"],
    ["90+ days", "days_90_plus"],
  ] as const;

  return (
    <>
      <div className="mb-5 grid gap-3 sm:grid-cols-3">
        <Stat label="Total owed" value={formatMoney(report.grand_total)} />
        <Stat
          label="Overdue 90+ days"
          value={formatMoney(report.totals.days_90_plus)}
          tone={report.totals.days_90_plus > 0 ? "danger" : undefined}
        />
        <Stat label="Not yet due" value={formatMoney(report.totals.current)} />
      </div>

      <div className="mb-3 flex justify-end">
        <Button
          onClick={() =>
            downloadCsv(
              `receivables-${report.as_of}.csv`,
              ["Client", "Total", ...cols.map(([label]) => label)],
              report.clients.map((c) => [
                c.client_name,
                c.total,
                ...cols.map(([, key]) => c.buckets[key]),
              ]),
            )
          }
        >
          Export CSV
        </Button>
      </div>

      <Card>
        {report.clients.length === 0 ? (
          <EmptyState
            title="Nothing outstanding"
            message="Every issued invoice has been paid."
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[46rem] text-sm">
              <thead>
                <tr className="border-b border-line text-left text-xs text-muted">
                  <th className="px-5 py-2.5 font-medium">Client</th>
                  {cols.map(([label]) => (
                    <th key={label} className="w-28 py-2.5 text-right font-medium">
                      {label}
                    </th>
                  ))}
                  <th className="w-32 px-5 py-2.5 text-right font-medium">Total</th>
                </tr>
              </thead>
              <tbody>
                {report.clients.map((c) => (
                  <tr key={c.client_id} className="border-b border-line">
                    <td className="px-5 py-2.5 font-medium">{c.client_name}</td>
                    {cols.map(([label, key]) => (
                      <td
                        key={label}
                        className={`tabular py-2.5 text-right ${
                          key === "days_90_plus" && c.buckets[key] > 0 ? "text-danger" : ""
                        }`}
                      >
                        {c.buckets[key] ? formatMoney(c.buckets[key]) : "—"}
                      </td>
                    ))}
                    <td className="tabular px-5 py-2.5 text-right font-medium">
                      {formatMoney(c.total)}
                    </td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr className="font-medium">
                  <td className="px-5 py-2.5">All clients</td>
                  {cols.map(([label, key]) => (
                    <td key={label} className="tabular py-2.5 text-right">
                      {formatMoney(report.totals[key])}
                    </td>
                  ))}
                  <td className="tabular px-5 py-2.5 text-right">
                    {formatMoney(report.grand_total)}
                  </td>
                </tr>
              </tfoot>
            </table>
          </div>
        )}
      </Card>
      <p className="mt-3 text-xs text-muted">As of {formatDate(report.as_of)}</p>
    </>
  );
}

/* ---------- GSTR-1 ---------- */

/** First and last day of the month a date falls in, as YYYY-MM-DD. */
function monthRange(d: Date) {
  const pad = (n: number) => String(n).padStart(2, "0");
  const first = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-01`;
  const lastDay = new Date(d.getFullYear(), d.getMonth() + 1, 0).getDate();
  return { start: first, end: `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(lastDay)}` };
}

function GSTTab({ companyId }: { companyId: number }) {
  const initial = useMemo(() => monthRange(new Date()), []);
  const [start, setStart] = useState(initial.start);
  const [end, setEnd] = useState(initial.end);
  const [report, setReport] = useState<GSTReport | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      setReport(await reportsApi.gstr1(companyId, start, end));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load the report");
    } finally {
      setLoading(false);
    }
  }, [companyId, start, end]);

  useEffect(() => {
    void (async () => {
      await load();
    })();
    // Only on mount and when Run is pressed — retyping a date should not fire a query
    // per keystroke.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <>
      <div className="mb-5 flex flex-wrap items-end gap-3">
        <div className="w-44">
          <Field label="From" type="date" value={start} onChange={(e) => setStart(e.target.value)} />
        </div>
        <div className="w-44">
          <Field label="To" type="date" value={end} onChange={(e) => setEnd(e.target.value)} />
        </div>
        <Button className="mb-4" variant="primary" loading={loading} onClick={() => void load()}>
          Run
        </Button>
        {report && (
          <Button
            className="mb-4"
            onClick={() =>
              downloadCsv(
                `gstr1-${report.start}-to-${report.end}.csv`,
                [
                  "Invoice", "Date", "Client", "GSTIN", "Place of supply",
                  "Taxable value", "CGST", "SGST", "IGST", "Total",
                ],
                report.invoices.map((i) => [
                  i.invoice_number, i.invoice_date, i.client_name, i.client_gstin,
                  i.place_of_supply, i.taxable_value, i.cgst, i.sgst, i.igst, i.total,
                ]),
              )
            }
          >
            Export B2B CSV
          </Button>
        )}
      </div>

      {error ? (
        <Card>
          <EmptyState
            title="Couldn't load the report"
            message={error}
            action={<Button onClick={() => void load()}>Try again</Button>}
          />
        </Card>
      ) : !report ? (
        <Card>
          <div className="grid place-items-center py-16">
            <Spinner />
          </div>
        </Card>
      ) : (
        <>
          <div className="mb-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Stat label="Invoices" value={String(report.summary.invoice_count)} />
            <Stat label="Taxable value" value={formatMoney(report.net_summary.taxable_value)} />
            <Stat
              label="CGST + SGST"
              value={formatMoney(report.net_summary.cgst + report.net_summary.sgst)}
            />
            <Stat label="IGST" value={formatMoney(report.net_summary.igst)} />
          </div>

          <p className="mb-5 text-xs text-muted">
            Figures are net of {report.credit_notes.length} credit note
            {report.credit_notes.length === 1 ? "" : "s"} in the period. Place of supply is
            compared against {report.company_state || "your company's state"} to decide
            CGST+SGST or IGST.
          </p>

          <Card className="mb-5 p-5">
            <p className="mb-3 text-sm font-medium">HSN summary</p>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[40rem] text-sm">
                <thead>
                  <tr className="border-b border-line text-left text-xs text-muted">
                    <th className="py-2 font-medium">HSN</th>
                    <th className="w-20 py-2 text-right font-medium">Rate</th>
                    <th className="w-20 py-2 text-right font-medium">Qty</th>
                    <th className="w-32 py-2 text-right font-medium">Taxable</th>
                    <th className="w-28 py-2 text-right font-medium">CGST</th>
                    <th className="w-28 py-2 text-right font-medium">SGST</th>
                    <th className="w-28 py-2 text-right font-medium">IGST</th>
                  </tr>
                </thead>
                <tbody>
                  {report.hsn_summary.map((h) => (
                    <tr key={`${h.hsn_code}-${h.tax_rate}`} className="border-b border-line">
                      <td className="py-2">{h.hsn_code || "—"}</td>
                      <td className="tabular py-2 text-right">{h.tax_rate}%</td>
                      <td className="tabular py-2 text-right">{h.total_qty}</td>
                      <td className="tabular py-2 text-right">{formatMoney(h.taxable_value)}</td>
                      <td className="tabular py-2 text-right">{formatMoney(h.cgst)}</td>
                      <td className="tabular py-2 text-right">{formatMoney(h.sgst)}</td>
                      <td className="tabular py-2 text-right">{formatMoney(h.igst)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Card>

          <Card className="p-5">
            <p className="mb-3 text-sm font-medium">Invoices</p>
            {report.invoices.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted">
                No invoices issued in this period.
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full min-w-[46rem] text-sm">
                  <thead>
                    <tr className="border-b border-line text-left text-xs text-muted">
                      <th className="py-2 font-medium">Invoice</th>
                      <th className="py-2 font-medium">Client</th>
                      <th className="w-32 py-2 font-medium">GSTIN</th>
                      <th className="w-32 py-2 text-right font-medium">Taxable</th>
                      <th className="w-28 py-2 text-right font-medium">Tax</th>
                      <th className="w-32 py-2 text-right font-medium">Total</th>
                    </tr>
                  </thead>
                  <tbody>
                    {report.invoices.map((i) => (
                      <tr key={i.invoice_id} className="border-b border-line">
                        <td className="py-2">
                          <p>{i.invoice_number}</p>
                          <p className="text-xs text-muted">{formatDate(i.invoice_date)}</p>
                        </td>
                        <td className="py-2">
                          <p className="truncate">{i.client_name}</p>
                          <p className="text-xs text-muted">{i.place_of_supply || "—"}</p>
                        </td>
                        <td className="py-2 text-xs">{i.client_gstin || "Unregistered"}</td>
                        <td className="tabular py-2 text-right">{formatMoney(i.taxable_value)}</td>
                        <td className="tabular py-2 text-right">
                          {formatMoney(i.cgst + i.sgst + i.igst)}
                        </td>
                        <td className="tabular py-2 text-right">{formatMoney(i.total)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        </>
      )}
    </>
  );
}
