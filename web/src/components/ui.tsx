"use client";

import { createContext, useCallback, useContext, useState } from "react";

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

/** Rendered once in the root layout; any screen can raise a toast through useToast. */
export function ToastHost() {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const show = useCallback<ToastApi["show"]>((message, tone = "info") => {
    const id = nextId++;
    setToasts((t) => [...t, { id, message, tone }]);
    window.setTimeout(
      () => setToasts((t) => t.filter((x) => x.id !== id)),
      tone === "error" ? 6000 : 3500,
    );
  }, []);

  // Published on window so non-React call sites (and useToast below) reach the same
  // instance without threading a provider through every page.
  if (typeof window !== "undefined") {
    (window as unknown as { __toast?: ToastApi }).__toast = { show };
  }

  return (
    <ToastCtx.Provider value={{ show }}>
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
  const ctx = useContext(ToastCtx);
  return {
    show: (message, tone) => {
      const global = (window as unknown as { __toast?: ToastApi }).__toast;
      (global ?? ctx).show(message, tone);
    },
  };
}
