"use client";

import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  clients as clientsApi,
  downloadPdf,
  formatDate,
  formatMoney,
  invoices as invoicesApi,
  today,
  type Client,
  type InvoiceDetail,
  type InvoiceRow,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { AppShell, NoCompany } from "@/components/AppShell";
import { DraftLine, LineItems, draftTotals, lineToInput } from "@/components/LineItems";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorText,
  Field,
  Modal,
  Select,
  Sheet,
  SkeletonRows,
  Spinner,
  TableWrap,
  Td,
  Th,
  useToast,
} from "@/components/ui";

const PAGE = 25;

/** Status drives what can be done to an invoice, so it is shown prominently. */
function StatusBadge({ status, overdue }: { status: string; overdue?: boolean }) {
  if (status === "paid") return <Badge tone="success">Paid</Badge>;
  if (status === "cancelled") return <Badge tone="neutral">Cancelled</Badge>;
  if (status === "draft") return <Badge tone="neutral">Draft</Badge>;
  if (overdue) return <Badge tone="danger">Overdue</Badge>;
  if (status === "partial") return <Badge tone="warning">Part paid</Badge>;
  return <Badge tone="info">Issued</Badge>;
}

export default function InvoicesPage() {
  const { company, loading: authLoading } = useAuth();
  const toast = useToast();

  const [rows, setRows] = useState<InvoiceRow[]>([]);
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<InvoiceDetail | null>(null);
  const [viewing, setViewing] = useState<number | null>(null);

  const companyId = company?.id ?? null;

  const load = useCallback(
    async (nextOffset: number, signal?: AbortSignal) => {
      if (!companyId) return;
      setLoading(true);
      setError("");
      try {
        const res = await invoicesApi.list(companyId, {
          limit: PAGE,
          offset: nextOffset,
          signal,
        });
        if (signal?.aborted) return;
        const data = res.data ?? [];
        setRows(data);
        setOffset(nextOffset);
        // The endpoint reports no total, so a full page is taken as "there may be more".
        setHasMore(data.length === PAGE);
      } catch (err) {
        if (signal?.aborted) return;
        setError(err instanceof ApiError ? err.message : "Failed to load invoices");
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [companyId],
  );

  useEffect(() => {
    if (!companyId) return;
    const controller = new AbortController();
    void (async () => {
      await load(0, controller.signal);
    })();
    return () => controller.abort();
  }, [companyId, load]);

  const reload = useCallback(() => void load(offset), [load, offset]);

  if (authLoading || !company) {
    return (
      <AppShell title="Invoices">{authLoading ? null : <NoCompany />}</AppShell>
    );
  }

  return (
    <AppShell
      title="Invoices"
      description="Drafts can be edited; issuing assigns the number and moves stock"
      actions={
        <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
          New invoice
        </Button>
      }
    >
      <Card>
        {loading && rows.length === 0 ? (
          <SkeletonRows />
        ) : error ? (
          <EmptyState
            icon="alert"
            title="Couldn't load invoices"
            message={error}
            action={<Button onClick={reload}>Try again</Button>}
          />
        ) : rows.length === 0 ? (
          <EmptyState
            icon="invoice"
            title="No invoices yet"
            message="Raise your first invoice — it starts as a draft you can edit before issuing."
            action={
              <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
                New invoice
              </Button>
            }
          />
        ) : (
          <ul className="divide-y divide-line">
            {rows.map((inv) => (
              <li
                key={inv.id}
                className="group flex flex-wrap items-center gap-x-4 gap-y-2 px-5 py-3.5 transition hover:bg-subtle/60"
              >
                <div className="min-w-0 flex-1 basis-full sm:basis-auto">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="truncate text-label-14 font-medium">
                      {inv.invoice_number || "Draft"}
                    </p>
                    <StatusBadge status={inv.status} overdue={inv.is_overdue} />
                  </div>
                  <p className="mt-0.5 truncate text-copy-13 text-muted">
                    {inv.client_name} · {formatDate(inv.invoice_date)}
                    {inv.is_overdue ? ` · ${inv.days_overdue} days overdue` : ""}
                  </p>
                </div>
                <div className="ml-auto text-right sm:w-32">
                  <p className="tabular text-label-14 font-medium">{formatMoney(inv.total)}</p>
                  {inv.remaining_amount > 0 && inv.status !== "cancelled" && (
                    <p className="tabular text-label-12 text-muted">
                      {formatMoney(inv.remaining_amount)} due
                    </p>
                  )}
                </div>
                <Button size="sm" onClick={() => setViewing(inv.id)}>
                  Open
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {(offset > 0 || hasMore) && (
        <div className="mt-4 flex items-center justify-between">
          <Button disabled={offset === 0} onClick={() => void load(Math.max(0, offset - PAGE))}>
            Previous
          </Button>
          <span className="text-label-12 text-muted">
            {offset + 1}–{offset + rows.length}
          </span>
          <Button disabled={!hasMore} onClick={() => void load(offset + PAGE)}>
            Next
          </Button>
        </div>
      )}

      {(creating || editing) && (
        <InvoiceForm
          companyId={company.id}
          invoice={editing}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
          onSaved={(number) => {
            setCreating(false);
            setEditing(null);
            toast.show(`${number} saved`, "success");
            reload();
          }}
        />
      )}

      {viewing !== null && (
        <InvoiceDetailModal
          id={viewing}
          onClose={() => setViewing(null)}
          onChanged={reload}
          onEdit={(detail) => {
            setViewing(null);
            setEditing(detail);
          }}
        />
      )}
    </AppShell>
  );
}

/* ---------- Create / edit ---------- */

function InvoiceForm({
  companyId,
  invoice,
  onClose,
  onSaved,
}: {
  companyId: number;
  invoice: InvoiceDetail | null;
  onClose: () => void;
  onSaved: (number: string) => void;
}) {
  const [clientList, setClientList] = useState<Client[]>([]);
  const [clientId, setClientId] = useState(invoice ? String(invoice.client.id) : "");
  const [invoiceDate, setInvoiceDate] = useState(invoice?.invoice_date ?? today());
  const [dueDate, setDueDate] = useState(invoice?.due_date ?? today());
  const [discount, setDiscount] = useState(String(invoice?.discount ?? 0));
  // Derived from the prop rather than an effect: the form is mounted afresh for each
  // invoice, so this runs exactly once and never has to re-sync.
  const [lines, setLines] = useState<DraftLine[]>(() =>
    (invoice?.items ?? []).map((l) => ({
      item_id: l.item_id,
      name: l.item_name || `Item #${l.item_id}`,
      qty: String(l.qty),
      rate: String(l.rate),
      discount: String(l.discount),
      tax_rate: String(l.tax_rate),
    })),
  );
  const [preview, setPreview] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await clientsApi.list(companyId);
        if (!cancelled) setClientList(res.clients ?? []);
      } catch {
        /* the picker stays empty; the error surfaces on save */
      }
      if (!invoice) {
        try {
          const res = await invoicesApi.numberPreview(companyId);
          if (!cancelled) setPreview(res.preview);
        } catch {
          /* the number is assigned on issue regardless */
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [companyId, invoice]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!clientId) {
      setError("Pick a client");
      return;
    }
    if (lines.length === 0) {
      setError("Add at least one line");
      return;
    }
    if (lines.some((l) => (Number(l.qty) || 0) <= 0)) {
      setError("Every line needs a quantity greater than zero");
      return;
    }
    setSaving(true);
    setError("");
    try {
      const payload = {
        client_id: Number(clientId),
        invoice_date: invoiceDate,
        due_date: dueDate,
        discount: Number(discount) || 0,
        items: lines.map(lineToInput),
      };
      if (invoice) {
        await invoicesApi.update(invoice.id, payload);
        onSaved(invoice.invoice_number || "Invoice");
      } else {
        const res = await invoicesApi.create({ ...payload, company_id: companyId });
        onSaved(res.invoice_number || "Invoice");
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save invoice");
      setSaving(false);
    }
  };

  const totals = draftTotals(lines, discount);

  return (
    <Modal
      open
      wide
      title={invoice ? `Edit ${invoice.invoice_number || "draft"}` : "New invoice"}
      onClose={onClose}
    >
      <form onSubmit={submit} noValidate>
        <ErrorText>{error}</ErrorText>

        {preview && (
          <p className="mb-4 text-label-12 text-muted">
            Will be numbered <span className="font-medium text-ink">{preview}</span> when
            issued.
          </p>
        )}

        <div className="grid gap-x-4 sm:grid-cols-3">
          <Select
            label="Client"
            value={clientId}
            onChange={(e) => setClientId(e.target.value)}
          >
            <option value="">Select a client</option>
            {clientList.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </Select>
          <Field
            label="Invoice date"
            type="date"
            value={invoiceDate}
            onChange={(e) => setInvoiceDate(e.target.value)}
          />
          <Field
            label="Due date"
            type="date"
            value={dueDate}
            onChange={(e) => setDueDate(e.target.value)}
          />
        </div>

        <LineItems
          companyId={companyId}
          lines={lines}
          onChange={setLines}
          discount={discount}
          onDiscountChange={setDiscount}
        />

        <div className="mt-6 flex justify-end gap-2">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={saving} disabled={totals.total < 0}>
            {invoice ? "Save changes" : "Save draft"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/* ---------- Detail ---------- */

function InvoiceDetailModal({
  id,
  onClose,
  onChanged,
  onEdit,
}: {
  id: number;
  onClose: () => void;
  onChanged: () => void;
  onEdit: (detail: InvoiceDetail) => void;
}) {
  const toast = useToast();
  const [detail, setDetail] = useState<InvoiceDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [emailing, setEmailing] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setDetail(await invoicesApi.get(id));
    } catch (err) {
      toast.show(err instanceof ApiError ? err.message : "Failed to load invoice", "error");
      onClose();
    } finally {
      setLoading(false);
    }
  }, [id, onClose, toast]);

  useEffect(() => {
    void (async () => {
      await load();
    })();
  }, [load]);

  const act = async (label: string, fn: () => Promise<unknown>, done: string) => {
    setBusy(label);
    try {
      await fn();
      toast.show(done, "success");
      await load();
      onChanged();
    } catch (err) {
      toast.show(err instanceof ApiError ? err.message : "That didn't work", "error");
    } finally {
      setBusy("");
    }
  };

  const issue = () =>
    act(
      "issue",
      async () => {
        try {
          await invoicesApi.issue(id);
        } catch (err) {
          // Refusing to issue past available stock is deliberate, so it is put to the
          // user rather than forced silently.
          if (
            err instanceof ApiError &&
            /stock/i.test(err.message) &&
            window.confirm(`${err.message}\n\nIssue anyway and let stock go negative?`)
          ) {
            await invoicesApi.issue(id, true);
            return;
          }
          throw err;
        }
      },
      "Invoice issued",
    );

  const footer = detail ? (
    <>
      <Button
        icon="download"
        onClick={() =>
          void downloadPdf(
            `/invoices/${detail.id}/pdf`,
            `${detail.invoice_number || "invoice"}.pdf`,
          ).catch((err) =>
            toast.show(err instanceof ApiError ? err.message : "Failed to download", "error"),
          )
        }
      >
        PDF
      </Button>

      {detail.status !== "draft" && detail.status !== "cancelled" && (
        <Button icon="mail" onClick={() => setEmailing(true)}>
          Email
        </Button>
      )}

      {detail.status === "draft" && (
        <>
          <Button icon="edit" onClick={() => onEdit(detail)}>
            Edit
          </Button>
          <Button
            variant="danger"
            icon="trash"
            loading={busy === "delete"}
            onClick={() => {
              if (!window.confirm("Delete this draft? This cannot be undone.")) return;
              void act("delete", () => invoicesApi.remove(detail.id), "Draft deleted").then(
                onClose,
              );
            }}
          >
            Delete
          </Button>
          <Button variant="primary" icon="check" loading={busy === "issue"} onClick={() => void issue()}>
            Issue
          </Button>
        </>
      )}

      {(detail.status === "issued" || detail.status === "partial") && (
        <Button
          variant="danger"
          loading={busy === "cancel"}
          onClick={() => {
            if (
              !window.confirm(
                "Cancel this invoice? It stays on record and stock is returned.",
              )
            )
              return;
            void act("cancel", () => invoicesApi.cancel(detail.id), "Invoice cancelled");
          }}
        >
          Cancel invoice
        </Button>
      )}
    </>
  ) : null;

  return (
    <>
      <Sheet
        open
        title={detail?.invoice_number || "Invoice"}
        subtitle={
          detail && (
            <span className="flex flex-wrap items-center gap-2">
              <StatusBadge status={detail.status} overdue={detail.is_overdue} />
              <span>
                {detail.client.name} · {formatDate(detail.invoice_date)} · due{" "}
                {formatDate(detail.due_date)}
              </span>
            </span>
          )
        }
        onClose={onClose}
        footer={footer}
      >
        {loading || !detail ? (
          <div className="grid place-items-center py-16 text-muted">
            <Spinner />
          </div>
        ) : (
          <>
            <TableWrap min="30rem">
              <thead>
                <tr>
                  <Th>Item</Th>
                  <Th align="right">Qty</Th>
                  <Th align="right">Rate</Th>
                  <Th align="right">GST %</Th>
                  <Th align="right">Amount</Th>
                </tr>
              </thead>
              <tbody>
                {detail.items.map((l) => (
                  <tr key={l.id}>
                    <Td>
                      <span className="font-medium">
                        {l.item_name || `Item #${l.item_id}`}
                      </span>
                      {l.hsn_code && (
                        <span className="ml-2 text-label-12 text-muted">HSN {l.hsn_code}</span>
                      )}
                    </Td>
                    <Td align="right" className="tabular">
                      {l.qty}
                    </Td>
                    <Td align="right" className="tabular">
                      {formatMoney(l.rate)}
                    </Td>
                    <Td align="right" className="tabular">
                      {l.tax_rate}
                    </Td>
                    <Td align="right" className="tabular">
                      {formatMoney(l.total)}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </TableWrap>

            <dl className="ml-auto mt-5 w-full max-w-xs space-y-1.5 text-sm">
              <Line label="Taxable value" value={formatMoney(detail.subtotal)} />
              {detail.discount > 0 && (
                <Line label="Discount" value={`− ${formatMoney(detail.discount)}`} />
              )}
              <Line label="GST" value={formatMoney(detail.tax)} />
              <div className="flex justify-between border-t border-line pt-2 font-semibold">
                <dt>Total</dt>
                <dd className="tabular">{formatMoney(detail.total)}</dd>
              </div>
              {detail.paid_amount > 0 && (
                <>
                  <Line label="Paid" value={formatMoney(detail.paid_amount)} />
                  <Line
                    label="Outstanding"
                    value={formatMoney(detail.remaining_amount)}
                  />
                </>
              )}
            </dl>
          </>
        )}
      </Sheet>

      {emailing && detail && (
        <EmailInvoiceModal
          invoice={detail}
          onClose={() => setEmailing(false)}
          onSent={() => {
            setEmailing(false);
            toast.show("Invoice emailed", "success");
          }}
        />
      )}
    </>
  );
}

/**
 * Emails the invoice PDF to the client.
 *
 * The address is pre-filled from the client's record but stays editable — an invoice
 * often has to go to an accounts inbox rather than the person who placed the order.
 * "Reminder" changes the wording the server sends for an invoice already overdue.
 */
function EmailInvoiceModal({
  invoice,
  onClose,
  onSent,
}: {
  invoice: InvoiceDetail;
  onClose: () => void;
  onSent: () => void;
}) {
  const { company } = useAuth();
  const [email, setEmail] = useState("");
  const [name, setName] = useState(invoice.client.name);
  const [reminder, setReminder] = useState(invoice.is_overdue);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!company) return;
    let cancelled = false;
    void (async () => {
      try {
        const res = await clientsApi.list(company.id);
        const match = (res.clients ?? []).find((c) => c.id === invoice.client.id);
        if (!cancelled && match?.email) setEmail(match.email);
      } catch {
        /* the address can be typed in */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [company, invoice.client.id]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim()) {
      setError("An email address is required");
      return;
    }
    setSending(true);
    setError("");
    try {
      await invoicesApi.sendEmail(invoice.id, {
        to_email: email.trim(),
        to_name: name.trim() || invoice.client.name,
        reminder,
      });
      onSent();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to send the email");
      setSending(false);
    }
  };

  return (
    <Modal
      open
      title={`Email ${invoice.invoice_number}`}
      description="Sends the invoice PDF as an attachment"
      onClose={onClose}
    >
      <form onSubmit={submit} noValidate>
        <ErrorText>{error}</ErrorText>
        <Field
          label="To"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="accounts@client.in"
          autoFocus
        />
        <Field label="Name" value={name} onChange={(e) => setName(e.target.value)} />
        <label className="mb-4 flex items-start gap-2.5 rounded-[var(--radius-base)] border border-line bg-subtle/50 px-3 py-2.5 text-sm">
          <input
            type="checkbox"
            checked={reminder}
            onChange={(e) => setReminder(e.target.checked)}
            className="mt-0.5"
          />
          <span>
            Send as a payment reminder
            <span className="mt-0.5 block text-label-12 text-muted">
              Wording for an invoice that is already due, rather than a first send.
            </span>
          </span>
        </label>
        <div className="flex justify-end gap-2">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" icon="mail" loading={sending}>
            Send
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function Line({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-3">
      <dt className="text-muted">{label}</dt>
      <dd className="tabular">{value}</dd>
    </div>
  );
}
