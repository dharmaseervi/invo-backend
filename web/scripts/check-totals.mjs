/**
 * Cross-checks src/lib/totals.ts against the Go implementation it mirrors.
 *
 * The web form previews an invoice total while the user types; the server computes the
 * real one. If the two ever disagree, the invoice appears to change value on save — so
 * this proves they agree to the paisa across randomised inputs.
 *
 *   cd .. && TOTALS_FIXTURE_OUT=/tmp/totals.json go test ./internal/handlers -run TestExportTotalsFixtures
 *   node scripts/check-totals.mjs /tmp/totals.json
 */
import { readFileSync } from "node:fs";
import { execSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";

const fixturePath = process.argv[2] ?? "/tmp/totals.json";
const outDir = join(tmpdir(), "invo-totals-check");

// typeRoots points at nothing on purpose: the app's tsconfig is for Next, and pulling
// in @types/node here fails to resolve under a bare tsc. Only this one file is compiled,
// and it has no dependencies.
execSync(
  `npx tsc src/lib/totals.ts --outDir ${outDir} --module es2022 --target es2022 --skipLibCheck --typeRoots ${outDir}`,
  { stdio: ["ignore", "ignore", "inherit"] },
);
const { computeTotals } = await import(join(outDir, "totals.js"));

const fixtures = JSON.parse(readFileSync(fixturePath, "utf8"));
let mismatches = 0;

for (const f of fixtures) {
  const got = computeTotals(
    f.items.map((i) => ({
      qty: i.qty,
      rate: i.rate,
      discount: i.discount,
      tax_rate: i.tax_rate,
    })),
    f.discount,
  );
  const checks = [
    ["subtotal", got.subtotal, f.subtotal],
    ["discount", got.discount, f.discount_out],
    ["tax", got.tax, f.tax],
    ["total", got.total, f.total],
    ...f.line_totals.map((v, i) => [`line ${i}`, got.lines[i].total, v]),
  ];
  for (const [name, ts, go] of checks) {
    if (Math.abs(ts - go) > 0.0001) {
      mismatches++;
      if (mismatches <= 5) {
        console.error(`MISMATCH ${name}: ts=${ts} go=${go}`, JSON.stringify(f.items), f.discount);
      }
    }
  }
}

if (mismatches) {
  console.error(`\n${fixtures.length} cases, ${mismatches} mismatches`);
  process.exit(1);
}
console.log(`${fixtures.length} cases, exact match`);
