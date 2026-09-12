"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ApiError,
  categories as categoriesApi,
  formatMoney,
  formatQty,
  items as itemsApi,
  type Category,
  type Item,
  type StockMovement,
} from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { AppShell, NoCompany } from "@/components/AppShell";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorText,
  Field,
  IconButton,
  Modal,
  SearchInput,
  Select,
  SkeletonRows,
  Spinner,
  TextArea,
  useToast,
} from "@/components/ui";

/** One request per screenful, near enough — the list is virtualised by the server. */
const PAGE_SIZE = 30;

export default function ItemsPage() {
  const { company, loading: authLoading } = useAuth();
  const toast = useToast();

  const [list, setList] = useState<Item[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [cats, setCats] = useState<Category[]>([]);
  const [editing, setEditing] = useState<Item | null>(null);
  const [creating, setCreating] = useState(false);
  const [stockFor, setStockFor] = useState<Item | null>(null);

  const companyId = company?.id ?? null;

  const catName = useMemo(() => {
    const map = new Map(cats.map((c) => [c.id, c.name]));
    return (id: number | null) => (id == null ? null : (map.get(id) ?? null));
  }, [cats]);

  const loadCategories = useCallback(async () => {
    if (!companyId) return;
    try {
      const res = await categoriesApi.list(companyId);
      setCats(res.categories ?? []);
    } catch {
      /* the list still renders without category names */
    }
  }, [companyId]);

  // First page. Debounced because `search` hits the database with an ILIKE, and
  // aborted on change so a slow reply for an abandoned query cannot overwrite the
  // results for the current one.
  const loadFirst = useCallback(
    async (term: string, signal: AbortSignal) => {
      if (!companyId) return;
      setLoading(true);
      setError("");
      try {
        const res = await itemsApi.list(companyId, {
          limit: PAGE_SIZE,
          search: term.trim() || undefined,
          signal,
        });
        if (signal.aborted) return;
        setList(res.items ?? []);
        setCursor(res.next_cursor);
      } catch (err) {
        if (signal.aborted) return;
        setError(err instanceof ApiError ? err.message : "Failed to load items");
      } finally {
        if (!signal.aborted) setLoading(false);
      }
    },
    [companyId],
  );

  useEffect(() => {
    if (!companyId) return;
    const controller = new AbortController();
    const t = window.setTimeout(
      () => void loadFirst(search, controller.signal),
      search ? 300 : 0,
    );
    return () => {
      window.clearTimeout(t);
      controller.abort();
    };
  }, [companyId, search, loadFirst]);

  useEffect(() => {
    void (async () => {
      await loadCategories();
    })();
  }, [loadCategories]);

  const loadMore = useCallback(async () => {
    if (!companyId || !cursor || loadingMore) return;
    setLoadingMore(true);
    try {
      const res = await itemsApi.list(companyId, {
        limit: PAGE_SIZE,
        cursor,
        search: search.trim() || undefined,
      });
      // Appending by id keeps a page that overlaps — an item renamed mid-scroll — from
      // showing twice.
      setList((prev) => {
        const seen = new Set(prev.map((i) => i.id));
        return [...prev, ...(res.items ?? []).filter((i) => !seen.has(i.id))];
      });
      setCursor(res.next_cursor);
    } catch (err) {
      toast.show(err instanceof ApiError ? err.message : "Failed to load more", "error");
    } finally {
      setLoadingMore(false);
    }
  }, [companyId, cursor, loadingMore, search, toast]);

  // Infinite scroll: a sentinel below the list asks for the next page as it comes
  // into view, which is the same behaviour as the iOS catalogue.
  const sentinel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const node = sentinel.current;
    if (!node || !cursor) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) void loadMore();
      },
      { rootMargin: "200px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [cursor, loadMore]);

  const reload = useCallback(() => {
    const controller = new AbortController();
    void loadFirst(search, controller.signal);
  }, [loadFirst, search]);

  if (authLoading || !company) {
    return (
      <AppShell title="Items">{authLoading ? null : <NoCompany />}</AppShell>
    );
  }

  return (
    <AppShell
      title="Items"
      description="Your catalogue, pricing and stock on hand"
      actions={
        <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
          Add item
        </Button>
      }
    >
      <div className="mb-4">
        <SearchInput
          value={search}
          onChange={setSearch}
          placeholder="Search name or SKU"
        />
      </div>

      <Card>
        {loading && list.length === 0 ? (
          <SkeletonRows />
        ) : error ? (
          <EmptyState
            icon="alert"
            title="Couldn't load items"
            message={error}
            action={<Button onClick={reload}>Try again</Button>}
          />
        ) : list.length === 0 ? (
          <EmptyState
            icon="item"
            title={search ? "No matches" : "No items yet"}
            message={
              search
                ? `Nothing matched "${search}".`
                : "Add the products and services you sell."
            }
            action={
              !search && (
                <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
                  Add item
                </Button>
              )
            }
          />
        ) : (
          <ul className="divide-y divide-line">
            {list.map((item) => (
              <li
                key={item.id}
                className="group flex flex-wrap items-center gap-x-4 gap-y-2 px-5 py-3.5 transition hover:bg-subtle/60"
              >
                <div className="min-w-0 flex-1 basis-full sm:basis-auto">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="truncate text-label-14 font-medium">{item.name}</p>
                    <StockBadge item={item} />
                  </div>
                  <p className="mt-0.5 truncate text-copy-13 text-muted">
                    {[
                      item.sku && `SKU ${item.sku}`,
                      catName(item.category_id),
                      item.hsn_code && `HSN ${item.hsn_code}`,
                    ]
                      .filter(Boolean)
                      .join(" · ") || "No SKU"}
                  </p>
                </div>

                <div className="ml-auto text-right sm:w-24">
                  <p className="tabular text-label-14 font-medium">{formatMoney(item.price)}</p>
                  <p className="text-label-12 text-muted">
                    {item.tax_rate > 0 ? `GST ${item.tax_rate}%` : "No GST"}
                  </p>
                </div>

                <div className="w-24 text-right">
                  <p className="tabular text-label-14 font-medium">
                    {formatQty(item.quantity)}
                  </p>
                  <p className="text-label-12 text-muted">{item.unit || "in stock"}</p>
                </div>

                <div className="flex items-center gap-1">
                  <Button size="sm" onClick={() => setStockFor(item)}>
                    Stock
                  </Button>
                  <IconButton icon="edit" label={`Edit ${item.name}`} onClick={() => setEditing(item)} />
                </div>
              </li>
            ))}
          </ul>
        )}

        {cursor && (
          <div ref={sentinel} className="grid place-items-center py-6">
            {loadingMore ? <Spinner /> : <Button onClick={() => void loadMore()}>Load more</Button>}
          </div>
        )}
      </Card>

      {list.length > 0 && !cursor && (
        <p className="mt-3 text-label-12 text-muted">
          All {list.length} item{list.length === 1 ? "" : "s"} loaded
        </p>
      )}

      {(creating || editing) && (
        <ItemForm
          companyId={company.id}
          item={editing}
          categories={cats}
          onCategoryAdded={loadCategories}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
          onSaved={(name) => {
            setCreating(false);
            setEditing(null);
            toast.show(`${name} saved`, "success");
            reload();
          }}
        />
      )}

      {stockFor && (
        <StockPanel
          item={stockFor}
          onClose={() => setStockFor(null)}
          onRestocked={() => {
            setStockFor(null);
            reload();
          }}
        />
      )}
    </AppShell>
  );
}

/** Out of stock takes precedence over low stock — it is the more urgent of the two. */
function StockBadge({ item }: { item: Item }) {
  if (item.quantity <= 0) return <Badge tone="danger">Out of stock</Badge>;
  if (item.low_stock_alert > 0 && item.quantity <= item.low_stock_alert)
    return <Badge tone="warning">Low stock</Badge>;
  return null;
}

/* ---------- Create / edit ---------- */

const BLANK = {
  name: "",
  sku: "",
  unit: "",
  description: "",
  hsn_code: "",
  cost_price: "",
  price: "",
  quantity: "",
  low_stock_alert: "",
  tax_rate: "",
};

/** Empty means zero here — a blank price field should not send NaN. */
function num(value: string): number {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

function ItemForm({
  companyId,
  item,
  categories,
  onCategoryAdded,
  onClose,
  onSaved,
}: {
  companyId: number;
  item: Item | null;
  categories: Category[];
  onCategoryAdded: () => Promise<void>;
  onClose: () => void;
  onSaved: (name: string) => void;
}) {
  const [form, setForm] = useState(() =>
    item
      ? {
          name: item.name,
          sku: item.sku,
          unit: item.unit,
          description: item.description,
          hsn_code: item.hsn_code,
          cost_price: String(item.cost_price ?? ""),
          price: String(item.price ?? ""),
          quantity: String(item.quantity ?? ""),
          low_stock_alert: String(item.low_stock_alert ?? ""),
          tax_rate: String(item.tax_rate ?? ""),
        }
      : BLANK,
  );
  const [categoryId, setCategoryId] = useState<string>(
    item?.category_id != null ? String(item.category_id) : "",
  );
  const [addingCategory, setAddingCategory] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const set =
    (key: keyof typeof BLANK) =>
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setForm((f) => ({ ...f, [key]: e.target.value }));

  // Picking a category fills in its defaults, but never overwrites something already
  // typed — the default exists to save keystrokes, not to correct the user.
  const pickCategory = (value: string) => {
    setCategoryId(value);
    const cat = categories.find((c) => String(c.id) === value);
    if (!cat) return;
    setForm((f) => ({
      ...f,
      hsn_code: f.hsn_code || (cat.default_hsn_code ?? ""),
      tax_rate: f.tax_rate || (cat.default_tax_rate != null ? String(cat.default_tax_rate) : ""),
    }));
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const name = form.name.trim();
    if (!name) {
      setError("Item name is required");
      return;
    }
    // The create endpoint rejects an item without a valid category outright, so this
    // is caught here rather than surfacing as a bare 403.
    if (!item && !categoryId) {
      setError("Pick a category — new items require one");
      return;
    }
    setSaving(true);
    setError("");
    try {
      const body = {
        name,
        category_id: categoryId ? Number(categoryId) : null,
        sku: form.sku.trim(),
        unit: form.unit.trim(),
        description: form.description.trim(),
        hsn_code: form.hsn_code.trim(),
        cost_price: num(form.cost_price),
        price: num(form.price),
        quantity: num(form.quantity),
        low_stock_alert: num(form.low_stock_alert),
        tax_rate: num(form.tax_rate),
        company_id: companyId,
      };
      if (item) await itemsApi.update(item.id, body);
      else await itemsApi.create(body);
      onSaved(name);
    } catch (err) {
      // A duplicate SKU comes back as 409 with a message naming the problem.
      setError(err instanceof ApiError ? err.message : "Failed to save item");
      setSaving(false);
    }
  };

  return (
    <>
      <Modal open title={item ? "Edit item" : "Add item"} onClose={onClose} wide>
        <form onSubmit={submit} noValidate>
          <ErrorText>{error}</ErrorText>

          <p className="mb-3 text-xs font-medium text-muted">Details</p>
          <Field label="Name" value={form.name} onChange={set("name")} autoFocus />

          <div className="grid gap-x-4 sm:grid-cols-2">
            <div>
              <Select
                label="Category"
                value={categoryId}
                onChange={(e) => pickCategory(e.target.value)}
              >
                <option value="">No category</option>
                {categories.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </Select>
              <button
                type="button"
                onClick={() => setAddingCategory(true)}
                className="-mt-2 mb-4 text-xs text-accent hover:underline"
              >
                New category
              </button>
            </div>
            <Field
              label="SKU"
              value={form.sku}
              onChange={set("sku")}
              hint="Must be unique within the company"
            />
          </div>

          <div className="grid gap-x-4 sm:grid-cols-2">
            <Field label="Unit" value={form.unit} onChange={set("unit")} placeholder="pcs, kg, box" />
            <Field label="HSN code" value={form.hsn_code} onChange={set("hsn_code")} />
          </div>

          <TextArea
            label="Description"
            rows={2}
            value={form.description}
            onChange={set("description")}
          />

          <p className="mb-3 mt-6 text-xs font-medium text-muted">Pricing and stock</p>
          <div className="grid gap-x-4 sm:grid-cols-3">
            <Field
              label="Selling price"
              type="number"
              step="0.01"
              min="0"
              value={form.price}
              onChange={set("price")}
            />
            <Field
              label="Cost price"
              type="number"
              step="0.01"
              min="0"
              value={form.cost_price}
              onChange={set("cost_price")}
            />
            <Field
              label="GST rate %"
              type="number"
              step="0.01"
              min="0"
              value={form.tax_rate}
              onChange={set("tax_rate")}
            />
          </div>

          <div className="grid gap-x-4 sm:grid-cols-2">
            <Field
              label="Quantity"
              type="number"
              step="1"
              value={form.quantity}
              onChange={set("quantity")}
              hint={item ? "Changing this is recorded as a stock adjustment" : undefined}
            />
            <Field
              label="Low stock alert at"
              type="number"
              step="1"
              min="0"
              value={form.low_stock_alert}
              onChange={set("low_stock_alert")}
            />
          </div>

          <div className="mt-6 flex justify-end gap-2">
            <Button type="button" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" loading={saving}>
              {item ? "Save changes" : "Add item"}
            </Button>
          </div>
        </form>
      </Modal>

      {addingCategory && (
        <CategoryForm
          companyId={companyId}
          onClose={() => setAddingCategory(false)}
          onSaved={async () => {
            setAddingCategory(false);
            await onCategoryAdded();
          }}
        />
      )}
    </>
  );
}

function CategoryForm({
  companyId,
  onClose,
  onSaved,
}: {
  companyId: number;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [hsn, setHsn] = useState("");
  const [rate, setRate] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setError("Category name is required");
      return;
    }
    setSaving(true);
    setError("");
    try {
      await categoriesApi.create({
        name: name.trim(),
        company_id: companyId,
        default_hsn_code: hsn.trim() || null,
        default_tax_rate: rate.trim() ? Number(rate) : null,
      });
      await onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create category");
      setSaving(false);
    }
  };

  return (
    <Modal open title="New category" onClose={onClose}>
      <form onSubmit={submit} noValidate>
        <ErrorText>{error}</ErrorText>
        <Field label="Name" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field
            label="Default HSN code"
            value={hsn}
            onChange={(e) => setHsn(e.target.value)}
            hint="Pre-filled on new items"
          />
          <Field
            label="Default GST rate %"
            type="number"
            step="0.01"
            min="0"
            value={rate}
            onChange={(e) => setRate(e.target.value)}
          />
        </div>
        <div className="mt-6 flex justify-end gap-2">
          <Button type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" loading={saving}>
            Create
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/* ---------- Stock ---------- */

const MOVEMENT_LABELS: Record<string, string> = {
  restock: "Restocked",
  sale: "Sold",
  adjustment: "Adjusted",
  initial: "Opening stock",
};

/**
 * Restocking and the audit trail, together: the reason a quantity changed matters as
 * much as the number, and seeing the history next to the form is what makes a wrong
 * count traceable rather than just correctable.
 */
function StockPanel({
  item,
  onClose,
  onRestocked,
}: {
  item: Item;
  onClose: () => void;
  onRestocked: () => void;
}) {
  const toast = useToast();
  const [qty, setQty] = useState("");
  const [reference, setReference] = useState("");
  const [note, setNote] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [movements, setMovements] = useState<StockMovement[]>([]);
  const [loadingMovements, setLoadingMovements] = useState(true);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await itemsApi.movements(item.id);
        if (!cancelled) setMovements(res.movements ?? []);
      } catch {
        /* the restock form still works without the history */
      } finally {
        if (!cancelled) setLoadingMovements(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [item.id]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const quantity = Number(qty);
    if (!Number.isFinite(quantity) || quantity <= 0) {
      setError("Enter how many units came in");
      return;
    }
    setSaving(true);
    setError("");
    try {
      const res = await itemsApi.restock(item.id, {
        quantity,
        reference: reference.trim() || undefined,
        note: note.trim() || undefined,
      });
      toast.show(`${item.name} now at ${formatQty(res.quantity)}`, "success");
      onRestocked();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to record stock");
      setSaving(false);
    }
  };

  return (
    <Modal open title={item.name} onClose={onClose} wide>
      <p className="mb-5 text-sm text-muted">
        In stock:{" "}
        <span className="tabular font-medium text-ink">
          {formatQty(item.quantity)} {item.unit}
        </span>
      </p>

      <form onSubmit={submit} noValidate className="mb-8">
        <p className="mb-3 text-xs font-medium text-muted">Record stock received</p>
        <ErrorText>{error}</ErrorText>
        <div className="grid gap-x-4 sm:grid-cols-3">
          <Field
            label="Quantity in"
            type="number"
            step="1"
            min="1"
            value={qty}
            onChange={(e) => setQty(e.target.value)}
            autoFocus
          />
          <Field
            label="Reference"
            value={reference}
            onChange={(e) => setReference(e.target.value)}
            placeholder="Supplier bill no."
          />
          <Field label="Note" value={note} onChange={(e) => setNote(e.target.value)} />
        </div>
        <div className="flex justify-end">
          <Button type="submit" variant="primary" loading={saving}>
            Add to stock
          </Button>
        </div>
      </form>

      <p className="mb-3 text-xs font-medium text-muted">Recent movements</p>
      {loadingMovements ? (
        <div className="grid place-items-center py-8">
          <Spinner />
        </div>
      ) : movements.length === 0 ? (
        <p className="py-6 text-center text-sm text-muted">
          No stock movements recorded yet.
        </p>
      ) : (
        <ul className="divide-y divide-line text-sm">
          {movements.map((m) => (
            <li key={m.id} className="flex items-center gap-4 py-2.5">
              <span className="flex-1 truncate">
                {MOVEMENT_LABELS[m.movement_type] ?? m.movement_type}
                {m.reference ? ` · ${m.reference}` : ""}
                {m.note ? ` · ${m.note}` : ""}
              </span>
              <span
                className={`tabular w-16 text-right font-medium ${
                  m.quantity_change < 0 ? "text-danger" : "text-success"
                }`}
              >
                {m.quantity_change > 0 ? "+" : ""}
                {formatQty(m.quantity_change)}
              </span>
              <span className="tabular w-16 text-right text-muted">
                {formatQty(m.new_quantity)}
              </span>
              <span className="w-44 text-right text-label-12 text-muted">{m.created_at}</span>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}
