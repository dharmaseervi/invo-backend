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

/* ---------- Buttons and inputs ---------- */

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "danger";
  loading?: boolean;
};

export function Button({
  variant = "secondary",
  loading = false,
  disabled,
  children,
  className = "",
  ...rest
}: ButtonProps) {
  const base =
    "inline-flex items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium transition disabled:opacity-50 disabled:cursor-not-allowed";
  const styles = {
    primary: "bg-accent text-accent-fg hover:brightness-110",
    secondary: "border border-line bg-surface text-ink hover:bg-subtle",
    danger: "border border-line text-danger hover:bg-danger/10",
  }[variant];

  return (
    <button
      {...rest}
      disabled={disabled || loading}
      className={`${base} ${styles} ${className}`}
    >
      {loading && <Spinner className="h-4 w-4" />}
      {children}
    </button>
  );
}

type FieldProps = React.InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  hint?: string;
};

export function Field({ label, hint, id, className = "", ...rest }: FieldProps) {
  const inputId = id ?? `f-${label.toLowerCase().replace(/\W+/g, "-")}`;
  return (
    <div className="mb-4">
      <label htmlFor={inputId} className="mb-1.5 block text-sm font-medium">
        {label}
      </label>
      <input
        {...rest}
        id={inputId}
        className={`w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-accent ${className}`}
      />
      {hint && <p className="mt-1.5 text-xs text-muted">{hint}</p>}
    </div>
  );
}

export function Spinner({ className = "h-5 w-5" }: { className?: string }) {
  return (
    <span
      role="status"
      aria-label="Loading"
      className={`inline-block animate-spin rounded-full border-2 border-line border-t-accent ${className}`}
    />
  );
}

export function Card({
  children,
  className = "",
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={`rounded-xl border border-line bg-surface shadow-sm ${className}`}
    >
      {children}
    </div>
  );
}

/** Inline error above a form. Kept visually distinct from a toast, which is transient. */
export function ErrorText({ children }: { children?: React.ReactNode }) {
  if (!children) return null;
  return (
    <p role="alert" className="mb-3 text-sm text-danger">
      {children}
    </p>
  );
}

/* ---------- Toasts ---------- */

type Toast = { id: number; message: string; tone: "info" | "error" | "success" };
type ToastApi = { show: (message: string, tone?: Toast["tone"]) => void };

const ToastCtx = createContext<ToastApi>({ show: () => {} });
let nextId = 1;

/**
 * Wraps the app so every screen shares one toast queue.
 *
 * It has to be a provider around `children` rather than a sibling of them: as a
 * sibling its context reached nothing, and the only way to raise a toast from a page
 * was a global on `window` — written during render, which React does not allow.
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

  return (
    <ToastCtx.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed bottom-5 right-5 z-50 flex flex-col gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            role="status"
            className={`pointer-events-auto max-w-sm rounded-lg border border-line bg-surface px-4 py-3 text-sm shadow-lg border-l-[3px] ${
              t.tone === "error"
                ? "border-l-danger"
                : t.tone === "success"
                  ? "border-l-success"
                  : "border-l-accent"
            }`}
          >
            {t.message}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export function useToast(): ToastApi {
  return useContext(ToastCtx);
}


/* ---------- More inputs ---------- */

type SelectProps = React.SelectHTMLAttributes<HTMLSelectElement> & {
  label: string;
  hint?: string;
};

export function Select({ label, hint, id, children, className = "", ...rest }: SelectProps) {
  const selectId = id ?? `s-${label.toLowerCase().replace(/\W+/g, "-")}`;
  return (
    <div className="mb-4">
      <label htmlFor={selectId} className="mb-1.5 block text-sm font-medium">
        {label}
      </label>
      <select
        {...rest}
        id={selectId}
        className={`w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-accent ${className}`}
      >
        {children}
      </select>
      {hint && <p className="mt-1.5 text-xs text-muted">{hint}</p>}
    </div>
  );
}

type TextAreaProps = React.TextareaHTMLAttributes<HTMLTextAreaElement> & {
  label: string;
};

export function TextArea({ label, id, className = "", ...rest }: TextAreaProps) {
  const areaId = id ?? `t-${label.toLowerCase().replace(/\W+/g, "-")}`;
  return (
    <div className="mb-4">
      <label htmlFor={areaId} className="mb-1.5 block text-sm font-medium">
        {label}
      </label>
      <textarea
        {...rest}
        id={areaId}
        className={`w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-accent ${className}`}
      />
    </div>
  );
}

/* ---------- Layout helpers ---------- */

/**
 * Modal dialog. Escape and a backdrop click both close it, and focus moves inside on
 * open so the keyboard lands in the form rather than behind it.
 */
export function Modal({
  open,
  title,
  onClose,
  children,
  wide = false,
}: {
  open: boolean;
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  wide?: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    // The body must not scroll behind the dialog, or a long list moves under it.
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    ref.current?.querySelector<HTMLElement>("input, select, textarea, button")?.focus();
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = previous;
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/40 p-4 sm:p-8"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={`w-full ${wide ? "max-w-3xl" : "max-w-lg"} rounded-xl border border-line bg-surface shadow-xl`}
      >
        <div className="flex items-center justify-between border-b border-line px-5 py-4">
          <h2 className="text-base font-semibold">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="rounded-lg px-2 py-1 text-lg leading-none text-muted hover:bg-subtle"
          >
            ×
          </button>
        </div>
        <div className="px-5 py-5">{children}</div>
      </div>
    </div>
  );
}

export function EmptyState({
  title,
  message,
  action,
}: {
  title: string;
  message: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="px-6 py-16 text-center">
      <p className="text-sm font-medium">{title}</p>
      <p className="mx-auto mt-1.5 max-w-sm text-sm text-muted">{message}</p>
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
}

export function Badge({
  tone = "muted",
  children,
}: {
  tone?: "muted" | "danger" | "warning" | "success";
  children: React.ReactNode;
}) {
  const styles = {
    muted: "bg-subtle text-muted",
    danger: "bg-danger/10 text-danger",
    warning: "bg-warning/15 text-warning",
    success: "bg-success/10 text-success",
  }[tone];
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${styles}`}
    >
      {children}
    </span>
  );
}

/** Search box with a clear button, used above every list. */
export function SearchInput({
  value,
  onChange,
  placeholder,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
}) {
  return (
    <div className="relative w-full sm:max-w-xs">
      <input
        type="search"
        role="searchbox"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-accent"
      />
    </div>
  );
}
