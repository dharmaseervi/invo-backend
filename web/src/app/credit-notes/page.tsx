"use client";

import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  clients as clientsApi,
  creditNotes as creditNotesApi,
  formatDate,
  formatMoney,
  invoices as invoicesApi,
  today,
  type Client,
  type CreditNoteDetail,
  type CreditNoteRow,
  type CreditNoteType,
  type InvoiceRow,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { AppShell, NoCompany } from "@/components/AppShell";
import { DraftLine, LineItems } from "@/components/LineItems";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorText,
  Field,
  Modal,
  Select,
  SkeletonRows,
  Spinner,
  useToast,
} from "@/components/ui";

export default function CreditNotesPage() {
  const { company, loading: authLoading } = useAuth();
  const toast = useToast();

  const [rows, setRows] = useState<CreditNoteRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const [viewing, setViewing] = useState<number | null>(null);

  const companyId = company?.id ?? null;

  const load = useCallback(async () => {
    if (!companyId) return;
    setLoading(true);
    setError("");
    try {
      const res = await creditNotesApi.list(companyId);
      setRows(res ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load credit notes");
    } finally {
      setLoading(false);
    }
  }, [companyId]);

  useEffect(() => {
    if (!companyId) return;
    void (async () => {
      await load();
    })();
  }, [companyId, load]);

  if (authLoading || !company) {
    return (
      <AppShell title="Credit notes">
        {authLoading ? null : <NoCompany />}
      </AppShell>
    );
  }

  return (
    <AppShell
      title="Credit notes"
      description="Reduces the GST owed on a supply that came back or was overcharged"
      actions={
        <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
          New credit note
        </Button>
      }
    >
      <Card>
        {loading && rows.length === 0 ? (
          <SkeletonRows />
        ) : error ? (
          <EmptyState
            icon="alert"
            title="Couldn't load credit notes"
            message={error}
            action={<Button onClick={() => void load()}>Try again</Button>}
          />
        ) : rows.length === 0 ? (
          <EmptyState
            icon="credit"
            title="No credit notes yet"
            message="Issue one when goods come back or a price was overcharged — it reduces the GST you owe on that supply."
            action={
              <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
                New credit note
              </Button>
            }
          />
        ) : (
          <ul className="divide-y divide-line">
            {rows.map((cn) => (
              <li
                key={cn.id}
                className="flex flex-wrap items-center gap-x-4 gap-y-2 px-5 py-3.5 transition hover:bg-subtle/60"
              >
                <div className="min-w-0 flex-1 basis-full sm:basis-auto">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="truncate text-label-14 font-medium">{cn.credit_number}</p>
                    <Badge tone={cn.type === "return" ? "neutral" : "warning"}>
                      {cn.type === "return"
                        ? "Goods returned"
                        : cn.type === "discount"
                          ? "Discount"
                          : "Adjustment"}
                    </Badge>
                  </div>
                  <p className="mt-0.5 truncate text-copy-13 text-muted">
                    {cn.client_name} · {formatDate(cn.credit_date)}
                  </p>
                </div>
                <div className="ml-auto text-right sm:w-32">
                  <p className="tabular text-label-14 font-medium">{formatMoney(cn.total)}</p>
                  {cn.balance > 0 && (
                    <p className="tabular text-label-12 text-muted">
                      {formatMoney(cn.balance)} unused
                    </p>
                  )}
                </div>
                <Button size="sm" onClick={() => setViewing(cn.id)}>
                  Open
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {creating && (
        <CreditNoteForm
          companyId={company.id}
          onClose={() => setCreating(false)}
          onSaved={() => {
            setCreating(false);
            toast.show("Credit note created", "success");
            void load();
          }}
        />
      )}

      {viewing !== null && (
        <CreditNoteDetailModal id={viewing} onClose={() => setViewing(null)} />
      )}
    </AppShell>
  );
}

function CreditNoteForm({
  companyId,
  onClose,
  onSaved,
}: {
  companyId: number;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [clientList, setClientList] = useState<Client[]>([]);
  const [clientInvoices, setClientInvoices] = useState<InvoiceRow[]>([]);
  const [clientId, setClientId] = useState("");
  const [invoiceId, setInvoiceId] = useState("");
  const [type, setType] = useState<CreditNoteType>("return");
  const [creditDate, setCreditDate] = useState(today());
  const [reason, setReason] = useState("");
  const [amount, setAmount] = useState("");
  const [lines, setLines] = useState<DraftLine[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await clientsApi.list(companyId);
        if (!cancelled) setClientList(res.clients ?? []);
      } catch {
        /* the error surfaces on save */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [companyId]);

  // Linking the note to its original invoice is what lets GSTR-1 report it in CDNR
  // against that supply, so the invoice list is offered as soon as a client is chosen.
  useEffect(() => {
    if (!clientId) return;
    let cancelled = false;
    void (async () => {
      try {
        const res = await invoicesApi.list(companyId, {
          limit: 50,
          clientId: Number(clientId),
        });
        if (!cancelled) setClientInvoices(res.data ?? []);
      } catch {
        if (!cancelled) setClientInvoices([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [clientId, companyId]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!clientId) {
      setError("Pick a client");
      return;
    }
    if (type === "return" && lines.length === 0) {
      setError("Add the items being returned");
      return;
    }
    if (type !== "return" && !(Number(amount) > 0)) {
      setError("Enter an amount greater than zero");
      return;
    }
    setSaving(true);
    setError("");
    try {
      await creditNotesApi.create({
        company_id: companyId,
        client_id: Number(clientId),
        invoice_id: invoiceId ? Number(invoiceId) : null,
        type,
        credit_date: creditDate,
        reason: reason.trim() || undefined,
        items:
          type === "return"
            ? lines.map((l) => ({
                item_id: l.item_id,
                qty: Number(l.qty) || 0,
                rate: Number(l.rate) || 0,
                tax_rate: Number(l.tax_rate) || 0,
              }))
            : undefined,
        amount: type === "return" ? undefined : Number(amount),
      });
      onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create credit note");
      setSaving(false);
    }
  };

  return (
    <Modal open wide title="New credit note" onClose={onClose}>
      <form onSubmit={submit} noValidate>
        <ErrorText>{error}</ErrorText>

        <div className="grid gap-x-4 sm:grid-cols-2">
          <Select label="Client" value={clientId} onChange={(e) => setClientId(e.target.value)}>
            <option value="">Select a client</option>
            {clientList.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </Select>
          <Select
            label="Against invoice"
            value={invoiceId}
            onChange={(e) => setInvoiceId(e.target.value)}
            hint="Optional, but required for the GST return to report it against that supply"
          >
            <option value="">Not linked to an invoice</option>
            {clientInvoices.map((i) => (
              <option key={i.id} value={i.id}>
                {i.invoice_number || `Draft #${i.id}`} · {formatMoney(i.total)}
              </option>
            ))}
          </Select>
        </div>

        <div className="grid gap-x-4 sm:grid-cols-2">
          <Select
            label="Type"
            value={type}
            onChange={(e) => setType(e.target.value as CreditNoteType)}
            hint={
              type === "return"
                ? "Goods come back into stock"
                : "Credits an amount without touching stock"
            }
          >
            <option value="return">Goods returned</option>
            <option value="adjustment">Adjustment</option>
            <option value="discount">Discount allowed later</option>
          </Select>
          <Field
            label="Credit date"
            type="date"
            value={creditDate}
            onChange={(e) => setCreditDate(e.target.value)}
          />
        </div>

        <Field
          label="Reason"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="Damaged in transit, overcharged, order cancelled"
        />

        {type === "return" ? (
          <LineItems
            companyId={companyId}
            lines={lines}
            onChange={setLines}
            discount="0"
            onDiscountChange={() => {}}
          />
        ) : (
          <Field
            label="Amount to credit"
            type="number"
            step="0.01"
            min="0"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
          />
        )}

        <div className="mt-6 flex justify-end gap-2">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={saving}>
            Create credit note
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function CreditNoteDetailModal({ id, onClose }: { id: number; onClose: () => void }) {
  const toast = useToast();
  const [detail, setDetail] = useState<CreditNoteDetail | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const res = await creditNotesApi.get(id);
        if (!cancelled) setDetail(res);
      } catch (err) {
        toast.show(
          err instanceof ApiError ? err.message : "Failed to load credit note",
          "error",
        );
        onClose();
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id, onClose, toast]);

  return (
    <Modal open wide title={detail?.credit_number || "Credit note"} onClose={onClose}>
      {loading || !detail ? (
        <div className="grid place-items-center py-16">
          <Spinner />
        </div>
      ) : (
        <>
          <p className="mb-5 text-sm text-muted">
            {detail.client_name} · {formatDate(detail.credit_date)}
            {detail.invoice_number ? ` · against ${detail.invoice_number}` : ""}
            {detail.reason ? ` · ${detail.reason}` : ""}
          </p>

          {detail.items.length > 0 && (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[30rem] text-sm">
                <thead>
                  <tr className="border-b border-line text-left text-label-12 text-muted">
                    <th className="py-2 font-medium">Item</th>
                    <th className="w-16 py-2 text-right font-medium">Qty</th>
                    <th className="w-24 py-2 text-right font-medium">Rate</th>
                    <th className="w-20 py-2 text-right font-medium">GST %</th>
                    <th className="w-28 py-2 text-right font-medium">Amount</th>
                  </tr>
                </thead>
                <tbody>
                  {detail.items.map((l) => (
                    <tr key={l.id} className="border-b border-line">
                      <td className="py-2">{l.item_name}</td>
                      <td className="tabular py-2 text-right">{l.qty}</td>
                      <td className="tabular py-2 text-right">{formatMoney(l.rate)}</td>
                      <td className="tabular py-2 text-right">{l.tax_rate}</td>
                      <td className="tabular py-2 text-right">{formatMoney(l.total)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <dl className="mt-5 ml-auto w-full max-w-xs space-y-1.5 text-sm">
            <div className="flex justify-between gap-3">
              <dt className="text-muted">Taxable value</dt>
              <dd className="tabular">{formatMoney(detail.subtotal)}</dd>
            </div>
            <div className="flex justify-between gap-3">
              <dt className="text-muted">GST</dt>
              <dd className="tabular">{formatMoney(detail.tax)}</dd>
            </div>
            <div className="flex justify-between border-t border-line pt-2 font-semibold">
              <dt>Total credited</dt>
              <dd className="tabular">{formatMoney(detail.total)}</dd>
            </div>
            <div className="flex justify-between gap-3">
              <dt className="text-muted">Unused balance</dt>
              <dd className="tabular">{formatMoney(detail.balance)}</dd>
            </div>
          </dl>
        </>
      )}
    </Modal>
  );
}
