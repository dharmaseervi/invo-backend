"use client";

import { useEffect, useState } from "react";

type Theme = "system" | "light" | "dark";
const KEY = "invo_theme";

function apply(theme: Theme) {
  const root = document.documentElement;
  if (theme === "system") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", theme);
}

/**
 * Light / dark / follow-the-system.
 *
 * Worth having rather than only following the OS: an invoice is often checked on a
 * bright shop counter on a laptop set to dark, and vice versa. The choice is stored
 * per browser; the inline script in the layout applies it before first paint so the
 * page never flashes the wrong palette.
 */
export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>("system");

  // Read after mount, not in a state initialiser: this page is prerendered at build
  // time, where there is no localStorage to read and a different answer would be a
  // hydration mismatch. The bootstrap script has already painted the right palette.
  useEffect(() => {
    void (async () => {
      try {
        const stored = window.localStorage.getItem(KEY) as Theme | null;
        if (stored === "light" || stored === "dark") setTheme(stored);
      } catch {
        /* storage unavailable — the system preference still applies */
      }
    })();
  }, []);

  const choose = (next: Theme) => {
    setTheme(next);
    apply(next);
    try {
      if (next === "system") window.localStorage.removeItem(KEY);
      else window.localStorage.setItem(KEY, next);
    } catch {
      /* the choice simply will not survive a reload */
    }
  };

  const options: { id: Theme; label: string; icon: "box" | "check" | "settings" }[] = [
    { id: "light", label: "Light", icon: "box" },
    { id: "dark", label: "Dark", icon: "box" },
    { id: "system", label: "System", icon: "settings" },
  ];

  return (
    <div
      role="radiogroup"
      aria-label="Colour theme"
      className="mb-1.5 flex rounded-lg border border-line bg-surface p-0.5"
    >
      {options.map((o) => (
        <button
          key={o.id}
          type="button"
          role="radio"
          aria-checked={theme === o.id}
          onClick={() => choose(o.id)}
          className={`flex-1 rounded-[7px] px-2 py-1 text-[11px] font-medium transition ${
            theme === o.id ? "bg-accent-soft text-accent" : "text-muted hover:text-ink"
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

/** Kept next to the toggle so the stored key lives in one place. */
export const THEME_BOOTSTRAP = `try{var t=localStorage.getItem('${KEY}');if(t==='light'||t==='dark'){document.documentElement.setAttribute('data-theme',t)}}catch(e){}`;
