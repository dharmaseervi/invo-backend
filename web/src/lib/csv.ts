/**
 * CSV export. Values are quoted and internal quotes doubled, per RFC 4180 — an item
 * name containing a comma would otherwise shift every later column in the row.
 *
 * A leading =, +, - or @ is prefixed with a single quote: spreadsheets treat such a
 * cell as a formula, and a product name starting with "=" would execute on open.
 */
function cell(value: unknown): string {
  if (value == null) return '""';
  const raw = String(value);
  const safe = /^[=+\-@]/.test(raw) ? `'${raw}` : raw;
  return `"${safe.replace(/"/g, '""')}"`;
}

export function toCsv(headers: string[], rows: unknown[][]): string {
  return [headers.map(cell).join(","), ...rows.map((r) => r.map(cell).join(","))].join("\r\n");
}

export function downloadCsv(filename: string, headers: string[], rows: unknown[][]) {
  // The BOM makes Excel read it as UTF-8; without it ₹ and Indian names are mangled.
  const blob = new Blob(["﻿" + toCsv(headers, rows)], {
    type: "text/csv;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
