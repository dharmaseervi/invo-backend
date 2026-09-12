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
import { Icon } from "@/components/icons";
import {
  Badge,
  Button,
  Card,
  CardHead,
  EmptyState,
  Field,
  SearchInput,
  Spinner,
  Stat,
  TableWrap,
  Tabs,
  Td,
  Th,
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
      <AppShell title="Reports">{authLoading ? null : <NoCompany />}</AppShell>
    );
  }

  return (
    <AppShell
      title="Reports"
      description="Stock on hand, who owes you, and what the GST return will say"
    >
      <div className="mb-5">
        <Tabs tabs={TABS} active={tab} onChange={setTab} />
      </div>

      {tab === "stock" && <StockTab companyId={company.id} />}
      {tab === "aging" && <AgingTab companyId={company.id} />}
      {tab === "gst" && <GSTTab companyId={company.id} />}
    </AppShell>
  );
}

/** Compact labelled dropdown for a toolbar, where a stacked label would misalign. */
function Filter({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
}) {
  return (
    <div className="relative">
      <select
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-10 appearance-none rounded-[var(--radius-base)] border border-line bg-surface pl-3 pr-8 text-label-14 outline-none focus:border-ink"
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
      <Icon
        name="chevron-down"
        className="pointer-events-none absolute right-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted"
      />
    </div>
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
        <Stat label="Stock at cost" value={formatMoney(report.total_cost_value)} icon="item" />
        <Stat label="Retail value" value={formatMoney(report.total_retail_value)} icon="report" />
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

      <Card className="mb-5">
        <CardHead title="By category" />
        <div className="p-2">
          <TableWrap min="38rem">
            <thead>
              <tr>
                <Th>Category</Th>
                <Th align="right">Items</Th>
                <Th align="right">Units</Th>
                <Th align="right">At cost</Th>
                <Th align="right">At retail</Th>
                <Th align="right">Share</Th>
              </tr>
            </thead>
            <tbody>
              {report.categories.map((c) => (
                <tr key={c.category_id ?? "none"} className="transition hover:bg-subtle/60">
                  <Td className="font-medium">{c.category_name}</Td>
                  <Td align="right" className="tabular">{c.item_count}</Td>
                  <Td align="right" className="tabular">{formatQty(c.total_units)}</Td>
                  <Td align="right" className="tabular">{formatMoney(c.cost_value)}</Td>
                  <Td align="right" className="tabular">{formatMoney(c.retail_value)}</Td>
                  <Td align="right" className="tabular text-muted">
                    {/* A bar makes the concentration obvious at a glance; the number
                        alone requires comparing six of them. */}
                    <span className="inline-flex items-center gap-2">
                      <span className="hidden h-1.5 w-12 overflow-hidden rounded-full bg-subtle sm:block">
                        <span
                          className="block h-full rounded-full bg-solid"
                          style={{ width: `${Math.min(c.share_of_value, 100)}%` }}
                        />
                      </span>
                      {c.share_of_value.toFixed(1)}%
                    </span>
                  </Td>
                </tr>
              ))}
            </tbody>
          </TableWrap>
        </div>
      </Card>

      <div className="mb-3 flex flex-wrap items-center gap-2">
        <SearchInput value={search} onChange={setSearch} placeholder="Search name or SKU" />
        <Filter
          label="Category"
          value={category}
          onChange={setCategory}
          options={[
            { value: "", label: "All categories" },
            ...report.categories.map((c) => ({
              value: String(c.category_id ?? ""),
              label: c.category_name,
            })),
          ]}
        />
        <Filter
          label="Status"
          value={status}
          onChange={setStatus}
          options={[
            { value: "", label: "All stock" },
            { value: "in", label: "In stock" },
            { value: "low", label: "Low" },
            { value: "out", label: "Out of stock" },
          ]}
        />
        <Button
          icon="download"
          className="ml-auto"
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
          Export
        </Button>
      </div>

      <Card>
        {rows.length === 0 ? (
          <EmptyState title="Nothing matches" message="Adjust the filters above." />
        ) : (
          <div className="p-2">
            <TableWrap min="42rem">
              <thead>
                <tr>
                  <Th>Item</Th>
                  <Th align="right">Qty</Th>
                  <Th align="right">At cost</Th>
                  <Th align="right">At retail</Th>
                  <Th align="right">Status</Th>
                </tr>
              </thead>
              <tbody>
                {rows.map((i) => (
                  <tr key={i.id} className="transition hover:bg-subtle/60">
                    <Td>
                      <p className="font-medium">{i.name}</p>
                      <p className="text-label-12 text-muted">
                        {[i.sku, i.category_name].filter(Boolean).join(" · ")}
                      </p>
                    </Td>
                    <Td align="right" className="tabular whitespace-nowrap">
                      {formatQty(i.quantity)} {i.unit}
                    </Td>
                    <Td align="right" className="tabular">{formatMoney(i.stock_value)}</Td>
                    <Td align="right" className="tabular">{formatMoney(i.retail_value)}</Td>
                    <Td align="right">
                      {i.status === "out" ? (
                        <Badge tone="danger">Out</Badge>
                      ) : i.status === "low" ? (
                        <Badge tone="warning">Low</Badge>
                      ) : (
                        <Badge tone="success">In stock</Badge>
                      )}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </TableWrap>
          </div>
        )}
      </Card>
      <p className="mt-3 text-label-12 text-muted">
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
        <Stat label="Total owed" value={formatMoney(report.grand_total)} icon="invoice" />
        <Stat
          label="Overdue 90+ days"
          value={formatMoney(report.totals.days_90_plus)}
          tone={report.totals.days_90_plus > 0 ? "danger" : undefined}
        />
        <Stat label="Not yet due" value={formatMoney(report.totals.current)} icon="payment" />
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
          <div className="p-2">
            <TableWrap min="44rem">
              <thead>
                <tr>
                  <Th>Client</Th>
                  {cols.map(([label]) => (
                    <Th key={label} align="right">
                      {label}
                    </Th>
                  ))}
                  <Th align="right">Total</Th>
                </tr>
              </thead>
              <tbody>
                {report.clients.map((c) => (
                  <tr key={c.client_id} className="transition hover:bg-subtle/60">
                    <Td className="font-medium">{c.client_name}</Td>
                    {cols.map(([label, key]) => (
                      <Td
                        key={label}
                        align="right"
                        className={`tabular ${
                          key === "days_90_plus" && c.buckets[key] > 0
                            ? "font-medium text-danger"
                            : c.buckets[key]
                              ? ""
                              : "text-muted"
                        }`}
                      >
                        {c.buckets[key] ? formatMoney(c.buckets[key]) : "—"}
                      </Td>
                    ))}
                    <Td align="right" className="tabular font-medium">
                      {formatMoney(c.total)}
                    </Td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr className="font-semibold">
                  <Td>All clients</Td>
                  {cols.map(([label, key]) => (
                    <Td key={label} align="right" className="tabular">
                      {formatMoney(report.totals[key])}
                    </Td>
                  ))}
                  <Td align="right" className="tabular">
                    {formatMoney(report.grand_total)}
                  </Td>
                </tr>
              </tfoot>
            </TableWrap>
          </div>
        )}
      </Card>
      <p className="mt-3 text-label-12 text-muted">As of {formatDate(report.as_of)}</p>
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
        <div className="w-40">
          <Field label="From" type="date" value={start} onChange={(e) => setStart(e.target.value)} className="!py-[7px]" />
        </div>
        <div className="w-40">
          <Field label="To" type="date" value={end} onChange={(e) => setEnd(e.target.value)} className="!py-[7px]" />
        </div>
        <Button className="mb-4" variant="primary" loading={loading} onClick={() => void load()}>
          Run
        </Button>
        {report && (
          <Button
            icon="download"
            className="mb-4 ml-auto"
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
            Export B2B
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
            <Stat label="Invoices" value={String(report.summary.invoice_count)} icon="invoice" />
            <Stat label="Taxable value" value={formatMoney(report.net_summary.taxable_value)} />
            <Stat
              label="CGST + SGST"
              value={formatMoney(report.net_summary.cgst + report.net_summary.sgst)}
            />
            <Stat label="IGST" value={formatMoney(report.net_summary.igst)} />
          </div>

          <p className="mb-5 text-label-12 text-muted">
            Figures are net of {report.credit_notes.length} credit note
            {report.credit_notes.length === 1 ? "" : "s"} in the period. Place of supply is
            compared against {report.company_state || "your company's state"} to decide
            CGST+SGST or IGST.
          </p>

          <Card className="mb-5">
            <CardHead title="HSN summary" />
            <div className="p-2">
              <TableWrap min="40rem">
                <thead>
                  <tr>
                    <Th>HSN</Th>
                    <Th align="right">Rate</Th>
                    <Th align="right">Qty</Th>
                    <Th align="right">Taxable</Th>
                    <Th align="right">CGST</Th>
                    <Th align="right">SGST</Th>
                    <Th align="right">IGST</Th>
                  </tr>
                </thead>
                <tbody>
                  {report.hsn_summary.map((h) => (
                    <tr key={`${h.hsn_code}-${h.tax_rate}`} className="transition hover:bg-subtle/60">
                      <Td className="font-medium">{h.hsn_code || "—"}</Td>
                      <Td align="right" className="tabular">{h.tax_rate}%</Td>
                      <Td align="right" className="tabular">{h.total_qty}</Td>
                      <Td align="right" className="tabular">{formatMoney(h.taxable_value)}</Td>
                      <Td align="right" className="tabular">{formatMoney(h.cgst)}</Td>
                      <Td align="right" className="tabular">{formatMoney(h.sgst)}</Td>
                      <Td align="right" className="tabular">{formatMoney(h.igst)}</Td>
                    </tr>
                  ))}
                </tbody>
              </TableWrap>
            </div>
          </Card>

          <Card>
            <CardHead title="Invoices" />
            {report.invoices.length === 0 ? (
              <EmptyState
                icon="invoice"
                title="Nothing in this period"
                message="No invoices were issued between these dates."
              />
            ) : (
              <div className="p-2">
                <TableWrap min="44rem">
                  <thead>
                    <tr>
                      <Th>Invoice</Th>
                      <Th>Client</Th>
                      <Th>GSTIN</Th>
                      <Th align="right">Taxable</Th>
                      <Th align="right">Tax</Th>
                      <Th align="right">Total</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {report.invoices.map((i) => (
                      <tr key={i.invoice_id} className="transition hover:bg-subtle/60">
                        <Td>
                          <p className="font-medium">{i.invoice_number}</p>
                          <p className="text-label-12 text-muted">{formatDate(i.invoice_date)}</p>
                        </Td>
                        <Td>
                          <p className="truncate">{i.client_name}</p>
                          <p className="text-label-12 text-muted">{i.place_of_supply || "—"}</p>
                        </Td>
                        <Td className="text-xs">
                          {i.client_gstin || <span className="text-muted">Unregistered</span>}
                        </Td>
                        <Td align="right" className="tabular">{formatMoney(i.taxable_value)}</Td>
                        <Td align="right" className="tabular">
                          {formatMoney(i.cgst + i.sgst + i.igst)}
                        </Td>
                        <Td align="right" className="tabular font-medium">{formatMoney(i.total)}</Td>
                      </tr>
                    ))}
                  </tbody>
                </TableWrap>
              </div>
            )}
          </Card>
        </>
      )}
    </>
  );
}
