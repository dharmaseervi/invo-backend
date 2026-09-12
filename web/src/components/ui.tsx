"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Icon, type IconName } from "@/components/icons";

/* ---------- Buttons ---------- */

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md";
  loading?: boolean;
  icon?: IconName;
};

export function Button({
  variant = "secondary",
  size = "md",
  loading = false,
  icon,
  disabled,
  children,
  className = "",
  ...rest
}: ButtonProps) {
  const base =
    "inline-flex shrink-0 items-center justify-center gap-1.5 rounded-[var(--radius-base)] font-medium transition-[background,border-color,color] duration-150 disabled:pointer-events-none disabled:opacity-45";
  const sizes = {
    sm: "h-8 px-2.5 text-label-13",
    md: "h-10 px-3.5 text-label-14",
  }[size];
  const variants = {
    // Geist reserves hue for meaning; the primary action is the highest-contrast fill
    // on the page, which is near-black in light and near-white in dark.
    primary: "bg-solid text-solid-fg hover:bg-solid-hover",
    secondary:
      "border border-line bg-surface text-ink hover:border-line-strong hover:bg-subtle",
    ghost: "text-muted hover:bg-subtle hover:text-ink",
    danger: "border border-line bg-surface text-danger hover:border-danger hover:bg-danger-soft",
  }[variant];

  return (
    <button
      {...rest}
      disabled={disabled || loading}
      className={`${base} ${sizes} ${variants} ${className}`}
    >
      {loading ? (
        <Spinner className="h-3.5 w-3.5" />
      ) : (
        icon && <Icon name={icon} className="h-4 w-4" />
      )}
      {children}
    </button>
  );
}

/** Square icon-only button, for row actions where a label would crowd the line. */
export function IconButton({
  icon,
  label,
  className = "",
  ...rest
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { icon: IconName; label: string }) {
  return (
    <button
      {...rest}
      aria-label={label}
      title={label}
      className={`inline-flex h-8 w-8 items-center justify-center rounded-[var(--radius-base)] text-muted transition hover:bg-subtle hover:text-ink ${className}`}
    >
      <Icon name={icon} className="h-4 w-4" />
    </button>
  );
}

/* ---------- Inputs ---------- */

// A Geist control is a hairline box: the border does the work, the focus state
// darkens it rather than adding a glow.
const fieldClass =
  "w-full rounded-[var(--radius-base)] border border-line bg-surface px-3 py-2 text-label-14 text-ink outline-none transition-colors placeholder:text-muted focus:border-ink disabled:opacity-60";

function Label({ htmlFor, children }: { htmlFor: string; children: React.ReactNode }) {
  return (
    <label htmlFor={htmlFor} className="mb-1.5 block text-label-13 font-medium text-ink">
      {children}
    </label>
  );
}

function Hint({ children }: { children?: React.ReactNode }) {
  if (!children) return null;
  return <p className="mt-1.5 text-copy-13 text-muted">{children}</p>;
}

type FieldProps = React.InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  hint?: React.ReactNode;
  /** Rendered inside the field, e.g. ₹ on a money input. */
  prefix?: string;
};

export function Field({ label, hint, id, prefix, className = "", ...rest }: FieldProps) {
  const inputId = id ?? `f-${label.toLowerCase().replace(/\W+/g, "-")}`;
  return (
    <div className="mb-4">
      <Label htmlFor={inputId}>{label}</Label>
      <div className="relative">
        {prefix && (
          <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted">
            {prefix}
          </span>
        )}
        <input
          {...rest}
          id={inputId}
          className={`${fieldClass} ${prefix ? "pl-7" : ""} ${className}`}
        />
      </div>
      <Hint>{hint}</Hint>
    </div>
  );
}

type SelectProps = React.SelectHTMLAttributes<HTMLSelectElement> & {
  label: string;
  hint?: React.ReactNode;
};

export function Select({ label, hint, id, children, className = "", ...rest }: SelectProps) {
  const selectId = id ?? `s-${label.toLowerCase().replace(/\W+/g, "-")}`;
  return (
    <div className="mb-4">
      <Label htmlFor={selectId}>{label}</Label>
      <div className="relative">
        <select
          {...rest}
          id={selectId}
          className={`${fieldClass} appearance-none pr-9 ${className}`}
        >
          {children}
        </select>
        <Icon
          name="chevron-down"
          className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted"
        />
      </div>
      <Hint>{hint}</Hint>
    </div>
  );
}

type TextAreaProps = React.TextareaHTMLAttributes<HTMLTextAreaElement> & { label: string };

export function TextArea({ label, id, className = "", ...rest }: TextAreaProps) {
  const areaId = id ?? `t-${label.toLowerCase().replace(/\W+/g, "-")}`;
  return (
    <div className="mb-4">
      <Label htmlFor={areaId}>{label}</Label>
      <textarea {...rest} id={areaId} className={`${fieldClass} resize-y ${className}`} />
    </div>
  );
}

export function SearchInput({
  value,
  onChange,
  placeholder,
  className = "",
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  className?: string;
}) {
  return (
    <div className={`relative w-full sm:max-w-xs ${className}`}>
      <Icon
        name="search"
        className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted"
      />
      <input
        type="search"
        role="searchbox"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={`${fieldClass} h-10 pl-9 [&::-webkit-search-cancel-button]:appearance-none`}
      />
      {value && (
        <button
          type="button"
          onClick={() => onChange("")}
          aria-label="Clear search"
          className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-muted hover:bg-subtle hover:text-ink"
        >
          <Icon name="close" className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  );
}

/* ---------- Feedback ---------- */

export function Spinner({ className = "h-5 w-5" }: { className?: string }) {
  return (
    <span
      role="status"
      aria-label="Loading"
      className={`inline-block animate-spin rounded-full border-2 border-current/25 border-t-current ${className}`}
    />
  );
}

export function Skeleton({ className = "h-4 w-full" }: { className?: string }) {
  return <div className={`skeleton ${className}`} aria-hidden="true" />;
}

/** Placeholder rows that hold the list's shape while it loads. */
export function SkeletonRows({ rows = 5 }: { rows?: number }) {
  return (
    <ul className="divide-y divide-line">
      {Array.from({ length: rows }).map((_, i) => (
        <li key={i} className="flex items-center gap-4 px-5 py-4">
          <div className="flex-1 space-y-2">
            <Skeleton className="h-3.5 w-40" />
            <Skeleton className="h-3 w-64" />
          </div>
          <Skeleton className="h-3.5 w-20" />
        </li>
      ))}
    </ul>
  );
}

export function ErrorText({ children }: { children?: React.ReactNode }) {
  if (!children) return null;
  return (
    <p
      role="alert"
      className="mb-4 flex items-start gap-2 rounded-[var(--radius-base)] border border-danger/30 bg-danger-soft px-3 py-2.5 text-copy-14 text-danger"
    >
      <Icon name="alert" className="mt-0.5 h-4 w-4 shrink-0" />
      <span>{children}</span>
    </p>
  );
}

/* ---------- Surfaces ---------- */

export function Card({
  children,
  className = "",
  as: Tag = "div",
}: {
  children: React.ReactNode;
  className?: string;
  as?: "div" | "section";
}) {
  return (
    <Tag
      className={`rounded-[var(--radius-card)] border border-line bg-surface ${className}`}
    >
      {children}
    </Tag>
  );
}

/** Card header with a title and optional trailing action. */
export function CardHead({
  title,
  action,
}: {
  title: React.ReactNode;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-line px-5 py-3.5">
      <h2 className="text-heading-16">{title}</h2>
      {action}
    </div>
  );
}

export function EmptyState({
  icon = "box",
  title,
  message,
  action,
}: {
  icon?: IconName;
  title: string;
  message: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="px-6 py-16 text-center">
      <div className="mx-auto mb-4 grid h-10 w-10 place-items-center rounded-[var(--radius-base)] border border-line bg-subtle text-muted">
        <Icon name={icon} className="h-4 w-4" />
      </div>
      <p className="text-heading-16">{title}</p>
      <p className="mx-auto mt-1.5 max-w-sm text-copy-14 text-muted">{message}</p>
      {action && <div className="mt-5 flex justify-center">{action}</div>}
    </div>
  );
}

type Tone = "neutral" | "danger" | "warning" | "success" | "accent" | "info";

/** Status pill. The dot carries the meaning at a glance; the word confirms it. */
export function Badge({
  tone = "neutral",
  children,
  dot = true,
}: {
  tone?: Tone;
  children: React.ReactNode;
  dot?: boolean;
}) {
  const styles: Record<Tone, string> = {
    neutral: "bg-subtle text-muted",
    danger: "bg-danger-soft text-danger",
    warning: "bg-warning-soft text-warning",
    success: "bg-success-soft text-success",
    accent: "bg-accent-soft text-accent",
    info: "bg-info-soft text-info",
  };
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-label-12 font-medium ${styles[tone]}`}
    >
      {dot && <span className="h-1.5 w-1.5 rounded-full bg-current opacity-80" />}
      {children}
    </span>
  );
}

/** Headline figure — the top row of every report and the dashboard. */
export function Stat({
  label,
  value,
  sub,
  tone,
  icon,
}: {
  label: string;
  value: string;
  sub?: React.ReactNode;
  tone?: "danger" | "warning" | "success";
  icon?: IconName;
}) {
  const valueTone =
    tone === "danger"
      ? "text-danger"
      : tone === "warning"
        ? "text-warning"
        : tone === "success"
          ? "text-success"
          : "";
  return (
    <Card className="p-4">
      <div className="flex items-start justify-between gap-2">
        <p className="text-label-13 text-muted">{label}</p>
        {icon && (
          <span className="text-muted">
            <Icon name={icon} className="h-4 w-4" />
          </span>
        )}
      </div>
      <p className={`tabular mt-2 text-[24px] font-medium leading-8 ${valueTone}`}>
        {value}
      </p>
      {sub && <div className="mt-1 text-label-12 text-muted">{sub}</div>}
    </Card>
  );
}

export function Tabs<T extends string>({
  tabs,
  active,
  onChange,
}: {
  tabs: { id: T; label: string }[];
  active: T;
  onChange: (id: T) => void;
}) {
  return (
    <div className="inline-flex rounded-[var(--radius-base)] border border-line bg-surface p-0.5">
      {tabs.map((t) => (
        <button
          key={t.id}
          type="button"
          onClick={() => onChange(t.id)}
          aria-current={active === t.id ? "page" : undefined}
          className={`rounded-[4px] px-3 py-1.5 text-label-13 font-medium transition ${
            active === t.id ? "bg-subtle text-ink" : "text-muted hover:text-ink"
          }`}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}

/* ---------- Tables ---------- */

/** Horizontal scroll lives on the table, never the page. */
export function TableWrap({
  children,
  min = "40rem",
}: {
  children: React.ReactNode;
  min?: string;
}) {
  return (
    <div className="scroll-slim overflow-x-auto">
      <table className="w-full text-sm" style={{ minWidth: min }}>
        {children}
      </table>
    </div>
  );
}

export function Th({
  children,
  align = "left",
  className = "",
}: {
  children?: React.ReactNode;
  align?: "left" | "right";
  className?: string;
}) {
  return (
    <th
      scope="col"
      className={`whitespace-nowrap border-b border-line px-3 py-2.5 text-label-12 font-medium text-muted ${
        align === "right" ? "text-right" : "text-left"
      } ${className}`}
    >
      {children}
    </th>
  );
}

export function Td({
  children,
  align = "left",
  className = "",
}: {
  children?: React.ReactNode;
  align?: "left" | "right";
  className?: string;
}) {
  return (
    <td
      className={`border-b border-line px-3 py-3 text-label-14 ${
        align === "right" ? "text-right" : ""
      } ${className}`}
    >
      {children}
    </td>
  );
}

/* ---------- Overlays ---------- */

function useOverlay(onClose: () => void) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key !== "Tab" || !ref.current) return;
      // Focus stays inside while it is open: tabbing out of a dialog and typing into
      // the page behind it is how data gets entered in the wrong place.
      const focusable = ref.current.querySelectorAll<HTMLElement>(
        'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])',
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    const previousOverflow = document.body.style.overflow;
    const previouslyFocused = document.activeElement as HTMLElement | null;
    document.body.style.overflow = "hidden";
    ref.current?.querySelector<HTMLElement>("input,select,textarea,button")?.focus();
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = previousOverflow;
      previouslyFocused?.focus?.();
    };
  }, [onClose]);

  return ref;
}

/** Centred dialog. Used for forms — something the user fills in and dismisses. */
export function Modal({
  open,
  title,
  description,
  onClose,
  children,
  wide = false,
}: {
  open: boolean;
  title: string;
  description?: string;
  onClose: () => void;
  children: React.ReactNode;
  wide?: boolean;
}) {
  const ref = useOverlay(onClose);
  if (!open) return null;

  return (
    <div
      className="animate-fade-in fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-ink/40 p-4 backdrop-blur-[2px] sm:p-8"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={`animate-pop-in w-full ${
          wide ? "max-w-3xl" : "max-w-lg"
        } rounded-[var(--radius-card)] border border-line bg-raised shadow-lg`}
      >
        <div className="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
          <div>
            <h2 className="text-heading-16">{title}</h2>
            {description && (
              <p className="mt-0.5 text-copy-13 text-muted">{description}</p>
            )}
          </div>
          <IconButton icon="close" label="Close" onClick={onClose} className="-mr-1" />
        </div>
        <div className="px-5 py-5">{children}</div>
      </div>
    </div>
  );
}

/**
 * Right-hand sheet. Used for inspecting a record — an invoice, an estimate — where the
 * list behind it is still the context the user is working in.
 */
export function Sheet({
  open,
  title,
  subtitle,
  onClose,
  children,
  footer,
}: {
  open: boolean;
  title: React.ReactNode;
  subtitle?: React.ReactNode;
  onClose: () => void;
  children: React.ReactNode;
  footer?: React.ReactNode;
}) {
  const ref = useOverlay(onClose);
  if (!open) return null;

  return (
    <div
      className="animate-fade-in fixed inset-0 z-40 flex justify-end bg-ink/40 backdrop-blur-[2px]"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === "string" ? title : "Details"}
        className="animate-slide-in flex h-full w-full max-w-2xl flex-col border-l border-line bg-raised"
      >
        <div className="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
          <div className="min-w-0">
            <h2 className="truncate text-heading-16">{title}</h2>
            {subtitle && <div className="mt-1.5 text-copy-13 text-muted">{subtitle}</div>}
          </div>
          <IconButton icon="close" label="Close" onClick={onClose} className="-mr-1" />
        </div>
        <div className="scroll-slim flex-1 overflow-y-auto px-5 py-5">{children}</div>
        {footer && (
          <div className="flex flex-wrap justify-end gap-2 border-t border-line bg-surface px-5 py-3.5">
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}

/* ---------- Toasts ---------- */

type Toast = { id: number; message: string; tone: "info" | "error" | "success" };
type ToastApi = { show: (message: string, tone?: Toast["tone"]) => void };

const ToastCtx = createContext<ToastApi>({ show: () => {} });
let nextId = 1;

/**
 * Wraps the app so every screen shares one toast queue. It has to be a provider around
 * `children` rather than a sibling of them: as a sibling its context reached nothing.
 */
export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const show = useCallback<ToastApi["show"]>((message, tone = "info") => {
    const id = nextId++;
    setToasts((t) => [...t, { id, message, tone }]);
    window.setTimeout(
      () => setToasts((t) => t.filter((x) => x.id !== id)),
      tone === "error" ? 6000 : 3500,
    );
  }, []);

  const value = useMemo<ToastApi>(() => ({ show }), [show]);

  const iconFor = { success: "check", error: "alert", info: "box" } as const;
  const toneClass = {
    success: "text-success",
    error: "text-danger",
    info: "text-accent",
  } as const;

  return (
    <ToastCtx.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed bottom-5 right-5 z-50 flex flex-col items-end gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            role="status"
            className="animate-pop-in pointer-events-auto flex max-w-sm items-start gap-2.5 rounded-[var(--radius-card)] border border-line bg-raised px-3.5 py-3 text-copy-14 shadow-lg"
          >
            <span className={`mt-0.5 ${toneClass[t.tone]}`}>
              <Icon name={iconFor[t.tone]} className="h-4 w-4" />
            </span>
            <span className="text-ink">{t.message}</span>
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export function useToast(): ToastApi {
  return useContext(ToastCtx);
}
