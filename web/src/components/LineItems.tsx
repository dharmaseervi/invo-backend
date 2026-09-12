"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { formatMoney, items as itemsApi, type Item, type LineInput } from "@/lib/api";
import { computeTotals, type Totals } from "@/lib/totals";
import { Spinner } from "@/components/ui";

/**
 * A line as it is being edited. Numbers are kept as strings so a half-typed "1." or an
 * emptied field stays as the user left it — coercing on every keystroke fights the
 * person typing.
 */
export type DraftLine = {
  item_id: number;
  name: string;
  qty: string;
  rate: string;
  discount: string;
  tax_rate: string;
};

export function lineToInput(l: DraftLine): LineInput {
  return {
    item_id: l.item_id,
    qty: Number(l.qty) || 0,
    rate: Number(l.rate) || 0,
    discount: Number(l.discount) || 0,
    tax_rate: Number(l.tax_rate) || 0,
  };
}

export function draftTotals(lines: DraftLine[], discount: string): Totals {
  return computeTotals(
    lines.map((l) => ({
      qty: Number(l.qty) || 0,
      rate: Number(l.rate) || 0,
      discount: Number(l.discount) || 0,
      tax_rate: Number(l.tax_rate) || 0,
    })),
    Number(discount) || 0,
  );
}

/**
 * Item search and the line table.
 *
 * Items are searched on the server rather than loaded in full: a catalogue of a
 * thousand products makes a dropdown useless and the payload large, and the same
 * search already backs the items screen.
 */
export function LineItems({
  companyId,
  lines,
  onChange,
  discount,
  onDiscountChange,
}: {
  companyId: number;
  lines: DraftLine[];
  onChange: (lines: DraftLine[]) => void;
  discount: string;
  onDiscountChange: (v: string) => void;
}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Item[]>([]);
  const [searching, setSearching] = useState(false);
  const boxRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const term = query.trim();
    const controller = new AbortController();
    const t = window.setTimeout(() => {
      void (async () => {
        if (!term) {
          setResults([]);
          return;
        }
        setSearching(true);
        try {
          const res = await itemsApi.list(companyId, {
            limit: 8,
            search: term,
            signal: controller.signal,
          });
          if (!controller.signal.aborted) setResults(res.items ?? []);
        } catch {
          /* a failed lookup leaves the previous results; the user can retype */
        } finally {
          if (!controller.signal.aborted) setSearching(false);
        }
      })();
    }, 250);
    return () => {
      window.clearTimeout(t);
      controller.abort();
    };
  }, [query, companyId]);

  const add = useCallback(
    (item: Item) => {
      // Rate and GST come from the item, but stay editable: a negotiated price on one
      // invoice should not require editing the catalogue.
      onChange([
        ...lines,
        {
          item_id: item.id,
          name: item.name,
          qty: "1",
          rate: String(item.price ?? 0),
          discount: "0",
          tax_rate: String(item.tax_rate ?? 0),
        },
      ]);
      setQuery("");
      setResults([]);
    },
    [lines, onChange],
  );

  const update = (index: number, key: keyof DraftLine, value: string) =>
    onChange(lines.map((l, i) => (i === index ? { ...l, [key]: value } : l)));

  const remove = (index: number) => onChange(lines.filter((_, i) => i !== index));

  const totals = draftTotals(lines, discount);

  return (
    <div>
      <div ref={boxRef} className="relative mb-4">
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search an item by name or SKU to add it"
          className="w-full rounded-[var(--radius-base)] border border-line bg-surface px-3 py-2 text-label-14 outline-none transition-colors focus:border-ink"
        />
        {query.trim() && (
          <div className="absolute z-10 mt-1 w-full overflow-hidden rounded-[var(--radius-card)] border border-line bg-raised shadow-lg">
            {searching && results.length === 0 ? (
              <div className="grid place-items-center py-4">
                <Spinner className="h-4 w-4" />
              </div>
            ) : results.length === 0 ? (
              <p className="px-3 py-3 text-sm text-muted">No items matched.</p>
            ) : (
              <ul className="max-h-64 overflow-y-auto">
                {results.map((item) => (
                  <li key={item.id}>
                    <button
                      type="button"
                      onClick={() => add(item)}
                      className="flex w-full items-center gap-3 px-3 py-2.5 text-left text-sm hover:bg-subtle"
                    >
                      <span className="min-w-0 flex-1 truncate">{item.name}</span>
                      <span className="text-xs text-muted">
                        {item.sku || "—"} · {item.quantity} in stock
                      </span>
                      <span className="tabular w-20 text-right">
                        {formatMoney(item.price)}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
      </div>

      {lines.length === 0 ? (
        <p className="rounded-lg border border-dashed border-line px-4 py-8 text-center text-sm text-muted">
          No lines yet. Search above to add the first one.
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[46rem] text-sm">
            <thead>
              <tr className="border-b border-line text-left text-label-12 text-muted">
                <th className="py-2 font-medium">Item</th>
                <th className="w-20 py-2 text-right font-medium">Qty</th>
                <th className="w-28 py-2 text-right font-medium">Rate</th>
                <th className="w-28 py-2 text-right font-medium">Discount</th>
                <th className="w-20 py-2 text-right font-medium">GST %</th>
                <th className="w-28 py-2 text-right font-medium">Amount</th>
                <th className="w-10 py-2" />
              </tr>
            </thead>
            <tbody>
              {lines.map((line, i) => (
                <tr key={`${line.item_id}-${i}`} className="border-b border-line">
                  <td className="py-2 pr-3">{line.name}</td>
                  <td className="py-2">
                    <NumCell
                      value={line.qty}
                      onChange={(v) => update(i, "qty", v)}
                      step="1"
                      label={`Quantity for ${line.name}`}
                    />
                  </td>
                  <td className="py-2">
                    <NumCell
                      value={line.rate}
                      onChange={(v) => update(i, "rate", v)}
                      label={`Rate for ${line.name}`}
                    />
                  </td>
                  <td className="py-2">
                    <NumCell
                      value={line.discount}
                      onChange={(v) => update(i, "discount", v)}
                      label={`Discount for ${line.name}`}
                    />
                  </td>
                  <td className="py-2">
                    <NumCell
                      value={line.tax_rate}
                      onChange={(v) => update(i, "tax_rate", v)}
                      label={`GST rate for ${line.name}`}
                    />
                  </td>
                  <td className="tabular py-2 pl-2 text-right">
                    {formatMoney(totals.lines[i]?.total ?? 0)}
                  </td>
                  <td className="py-2 text-right">
                    <button
                      type="button"
                      onClick={() => remove(i)}
                      aria-label={`Remove ${line.name}`}
                      className="rounded px-2 py-1 text-muted hover:bg-subtle hover:text-danger"
                    >
                      ×
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="mt-5 flex justify-end">
        <dl className="w-full max-w-xs space-y-1.5 text-sm">
          <Row label="Taxable value" value={formatMoney(totals.subtotal)} />
          <div className="flex items-center justify-between gap-3">
            <dt className="text-muted">Discount</dt>
            <dd>
              <input
                type="number"
                step="0.01"
                min="0"
                value={discount}
                onChange={(e) => onDiscountChange(e.target.value)}
                aria-label="Invoice discount"
                className="tabular w-28 rounded-[var(--radius-base)] border border-line bg-surface px-2 py-1 text-right outline-none transition-colors focus:border-ink"
              />
            </dd>
          </div>
          <Row label="GST" value={formatMoney(totals.tax)} />
          <div className="flex items-center justify-between border-t border-line pt-2 font-semibold">
            <dt>Total</dt>
            <dd className="tabular">{formatMoney(totals.total)}</dd>
          </div>
          {totals.discount > 0 && (
            <p className="pt-1 text-xs text-muted">
              The discount is spread across the lines before GST, so tax is charged on
              the discounted value.
            </p>
          )}
        </dl>
      </div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <dt className="text-muted">{label}</dt>
      <dd className="tabular">{value}</dd>
    </div>
  );
}

function NumCell({
  value,
  onChange,
  step = "0.01",
  label,
}: {
  value: string;
  onChange: (v: string) => void;
  step?: string;
  label: string;
}) {
  return (
    <input
      type="number"
      step={step}
      min="0"
      value={value}
      aria-label={label}
      onChange={(e) => onChange(e.target.value)}
      className="tabular w-full rounded-[var(--radius-base)] border border-line bg-surface px-2 py-1 text-right outline-none transition-colors focus:border-ink"
    />
  );
}
