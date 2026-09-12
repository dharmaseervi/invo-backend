"use client";

import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  clients as clientsApi,
  downloadPdf,
  estimates as estimatesApi,
  formatDate,
  formatMoney,
  today,
  type Client,
  type EstimateDetail,
  type EstimateRow,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { AppShell, NoCompany } from "@/components/AppShell";
import { DraftLine, LineItems, lineToInput } from "@/components/LineItems";
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

function StatusBadge({ status, converted }: { status: string; converted: boolean }) {
  if (converted) return <Badge tone="success">Invoiced</Badge>;
  if (status === "accepted") return <Badge tone="success">Accepted</Badge>;
  if (status === "rejected") return <Badge tone="danger">Rejected</Badge>;
  if (status === "sent") return <Badge tone="info">Sent</Badge>;
  return <Badge tone="neutral">Draft</Badge>;
}

export default function EstimatesPage() {
  const { company, loading: authLoading } = useAuth();
  const toast = useToast();

  const [rows, setRows] = useState<EstimateRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<EstimateDetail | null>(null);
  const [viewing, setViewing] = useState<number | null>(null);

  const companyId = company?.id ?? null;

  const load = useCallback(
    async (signal?: AbortSignal) => {
      if (!companyId) return;
      setLoading(true);
      setError("");
      try {
        const res = await estimatesApi.list(companyId, { signal });
        if (signal?.aborted) return;
        setRows(res.data ?? []);
      } catch (err) {
        if (signal?.aborted) return;
        setError(err instanceof ApiError ? err.message : "Failed to load estimates");
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
      await load(controller.signal);
    })();
    return () => controller.abort();
  }, [companyId, load]);

  const reload = useCallback(() => void load(), [load]);

  if (authLoading || !company) {
    return (
      <AppShell title="Estimates">{authLoading ? null : <NoCompany />}</AppShell>
    );
  }

  return (
    <AppShell
      title="Estimates"
      description="Quote a price first — an accepted estimate converts to an invoice"
      actions={
        <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
          New estimate
        </Button>
      }
    >
      <Card>
        {loading && rows.length === 0 ? (
          <SkeletonRows />
        ) : error ? (
          <EmptyState
            icon="alert"
            title="Couldn't load estimates"
            message={error}
            action={<Button onClick={reload}>Try again</Button>}
          />
        ) : rows.length === 0 ? (
          <EmptyState
            icon="estimate"
            title="No estimates yet"
            message="Quote a price before committing to an invoice — an accepted estimate converts to one."
            action={
              <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
                New estimate
              </Button>
            }
          />
        ) : (
          <ul className="divide-y divide-line">
            {rows.map((est) => (
              <li
                key={est.id}
                className="group flex flex-wrap items-center gap-x-4 gap-y-2 px-5 py-3.5 transition hover:bg-subtle/60"
              >
                <div className="min-w-0 flex-1 basis-full sm:basis-auto">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="truncate text-label-14 font-medium">{est.estimate_number}</p>
                    <StatusBadge
                      status={est.status}
                      converted={est.converted_invoice_id != null}
                    />
                  </div>
                  <p className="mt-0.5 truncate text-copy-13 text-muted">
                    {est.client_name} · {formatDate(est.estimate_date)}
                    {est.expiry_date ? ` · expires ${formatDate(est.expiry_date)}` : ""}
                  </p>
                </div>
                <p className="tabular ml-auto text-right text-label-14 font-medium sm:w-32">
                  {formatMoney(est.total)}
                </p>
                <Button size="sm" onClick={() => setViewing(est.id)}>
                  Open
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {(creating || editing) && (
        <EstimateForm
          companyId={company.id}
          estimate={editing}
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
        <EstimateDetailModal
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

function EstimateForm({
  companyId,
  estimate,
  onClose,
  onSaved,
}: {
  companyId: number;
  estimate: EstimateDetail | null;
  onClose: () => void;
  onSaved: (number: string) => void;
}) {
  const [clientList, setClientList] = useState<Client[]>([]);
  const [clientId, setClientId] = useState(estimate ? String(estimate.client.id) : "");
  const [estimateDate, setEstimateDate] = useState(estimate?.estimate_date ?? today());
  const [expiryDate, setExpiryDate] = useState(estimate?.expiry_date ?? "");
  const [discount, setDiscount] = useState(String(estimate?.discount ?? 0));
  const [lines, setLines] = useState<DraftLine[]>(() =>
    (estimate?.items ?? []).map((l) => ({
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
        /* the picker stays empty; saving still reports the real error */
      }
      if (!estimate) {
        try {
          const res = await estimatesApi.numberPreview(companyId);
          if (!cancelled) setPreview(res.preview);
        } catch {
          /* the number is assigned on save regardless */
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [companyId, estimate]);

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
    setSaving(true);
    setError("");
    try {
      const payload = {
        client_id: Number(clientId),
        estimate_date: estimateDate,
        // An empty date input is no expiry, not an empty string the server has to parse.
        expiry_date: expiryDate || null,
        discount: Number(discount) || 0,
        items: lines.map(lineToInput),
      };
      if (estimate) {
        await estimatesApi.update(estimate.id, payload);
        onSaved(estimate.estimate_number);
      } else {
        const res = await estimatesApi.create({ ...payload, company_id: companyId });
        onSaved(res.estimate_number);
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save estimate");
      setSaving(false);
    }
  };

  return (
    <Modal
      open
      wide
      title={estimate ? `Edit ${estimate.estimate_number}` : "New estimate"}
      onClose={onClose}
    >
      <form onSubmit={submit} noValidate>
        <ErrorText>{error}</ErrorText>
        {preview && (
          <p className="mb-4 text-label-12 text-muted">
            Will be numbered <span className="font-medium text-ink">{preview}</span>.
          </p>
        )}

        <div className="grid gap-x-4 sm:grid-cols-3">
          <Select label="Client" value={clientId} onChange={(e) => setClientId(e.target.value)}>
            <option value="">Select a client</option>
            {clientList.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </Select>
          <Field
            label="Estimate date"
            type="date"
            value={estimateDate}
            onChange={(e) => setEstimateDate(e.target.value)}
          />
          <Field
            label="Expires"
            type="date"
            value={expiryDate}
            onChange={(e) => setExpiryDate(e.target.value)}
            hint="Optional"
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
          <Button type="submit" variant="primary" loading={saving}>
            {estimate ? "Save changes" : "Create estimate"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function EstimateDetailModal({
  id,
  onClose,
  onChanged,
  onEdit,
}: {
  id: number;
  onClose: () => void;
  onChanged: () => void;
  onEdit: (detail: EstimateDetail) => void;
}) {
  const toast = useToast();
  const [detail, setDetail] = useState<EstimateDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setDetail(await estimatesApi.get(id));
    } catch (err) {
      toast.show(err instanceof ApiError ? err.message : "Failed to load estimate", "error");
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

  return (
    <Modal open wide title={detail?.estimate_number || "Estimate"} onClose={onClose}>
      {loading || !detail ? (
        <div className="grid place-items-center py-16">
          <Spinner />
        </div>
      ) : (
        <>
          <div className="mb-5 flex flex-wrap items-center gap-3">
            <StatusBadge
              status={detail.status}
              converted={detail.converted_invoice_id != null}
            />
            <span className="text-sm text-muted">
              {detail.client.name} · {formatDate(detail.estimate_date)}
            </span>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full min-w-[34rem] text-sm">
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
                    <td className="py-2">{l.item_name || `Item #${l.item_id}`}</td>
                    <td className="tabular py-2 text-right">{l.qty}</td>
                    <td className="tabular py-2 text-right">{formatMoney(l.rate)}</td>
                    <td className="tabular py-2 text-right">{l.tax_rate}</td>
                    <td className="tabular py-2 text-right">{formatMoney(l.total)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <dl className="mt-5 ml-auto w-full max-w-xs space-y-1.5 text-sm">
            <div className="flex justify-between gap-3">
              <dt className="text-muted">Taxable value</dt>
              <dd className="tabular">{formatMoney(detail.subtotal)}</dd>
            </div>
            {detail.discount > 0 && (
              <div className="flex justify-between gap-3">
                <dt className="text-muted">Discount</dt>
                <dd className="tabular">− {formatMoney(detail.discount)}</dd>
              </div>
            )}
            <div className="flex justify-between gap-3">
              <dt className="text-muted">GST</dt>
              <dd className="tabular">{formatMoney(detail.tax)}</dd>
            </div>
            <div className="flex justify-between border-t border-line pt-2 font-semibold">
              <dt>Total</dt>
              <dd className="tabular">{formatMoney(detail.total)}</dd>
            </div>
          </dl>

          <div className="mt-6 flex flex-wrap justify-end gap-2">
            <Button
              onClick={() =>
                void downloadPdf(
                  `/estimates/${detail.id}/pdf`,
                  `${detail.estimate_number}.pdf`,
                ).catch((err) =>
                  toast.show(
                    err instanceof ApiError ? err.message : "Failed to download",
                    "error",
                  ),
                )
              }
            >
              Download PDF
            </Button>

            {detail.converted_invoice_id == null && (
              <>
                <Button onClick={() => onEdit(detail)}>Edit</Button>
                {detail.status !== "sent" && detail.status !== "accepted" && (
                  <Button
                    loading={busy === "sent"}
                    onClick={() =>
                      void act("sent", () => estimatesApi.setStatus(detail.id, "sent"), "Marked as sent")
                    }
                  >
                    Mark sent
                  </Button>
                )}
                {detail.status !== "accepted" && (
                  <Button
                    loading={busy === "accepted"}
                    onClick={() =>
                      void act(
                        "accepted",
                        () => estimatesApi.setStatus(detail.id, "accepted"),
                        "Marked as accepted",
                      )
                    }
                  >
                    Mark accepted
                  </Button>
                )}
                {detail.status !== "rejected" && (
                  <Button
                    variant="danger"
                    loading={busy === "rejected"}
                    onClick={() =>
                      void act(
                        "rejected",
                        () => estimatesApi.setStatus(detail.id, "rejected"),
                        "Marked as rejected",
                      )
                    }
                  >
                    Mark rejected
                  </Button>
                )}
                <Button
                  variant="primary"
                  loading={busy === "convert"}
                  onClick={() =>
                    void act(
                      "convert",
                      async () => {
                        const res = await estimatesApi.convert(detail.id);
                        toast.show(`Created draft ${res.invoice_number}`, "success");
                      },
                      "Converted to an invoice",
                    )
                  }
                >
                  Convert to invoice
                </Button>
              </>
            )}
          </div>

          {detail.converted_invoice_id != null && (
            <p className="mt-4 text-right text-label-12 text-muted">
              Already converted to invoice #{detail.converted_invoice_id}.
            </p>
          )}
        </>
      )}
    </Modal>
  );
}
