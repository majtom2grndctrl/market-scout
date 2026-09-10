// Guards the failure mode the colour rename introduced: Tailwind emits nothing
// for a colour it cannot find and reports no error, so a stale `bg-card` (or a
// typo) renders unstyled and the build stays green. Every colour utility in
// source must name a token that exists. Tailwind emits
// nothing for an unknown colour and reports no error, so this is otherwise silent.
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../..", import.meta.url));
process.chdir(root);

const theme = readFileSync("app/theme.css", "utf8");
const defined = new Set([...theme.matchAll(/--color-([a-z0-9-]+):/g)].map((m) => m[1]));

// Tailwind's own palette and keywords are legitimate too.
const BUILTIN = /^(inherit|current|transparent|black|white|slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)(-\d+)?$/;
const PROPS = "bg|text|border|fill|stroke|ring|outline|divide|caret|decoration|placeholder|accent|shadow|from|to|via";

// shadcn's palette, replaced by the semantic families. These resolve to nothing
// and are the most likely thing to reappear via `shadcn add`.
const RETIRED = new Set([
  "background", "foreground", "card", "card-foreground", "popover", "popover-foreground",
  "primary", "primary-foreground", "secondary", "secondary-foreground",
  "muted", "muted-foreground", "accent", "accent-foreground",
  "destructive", "destructive-foreground", "border", "input", "ring",
  "chart-1", "chart-2", "chart-3", "chart-4", "chart-5",
]);

const files = [];
(function walk(dir) {
  for (const e of readdirSync(dir)) {
    if (e === "node_modules" || e.startsWith(".next")) continue;
    const p = join(dir, e);
    if (statSync(p).isDirectory()) walk(p);
    else if (/\.(tsx?|css)$/.test(p)) files.push(p);
  }
})(".");

const bad = new Map();
for (const f of files) {
  if (f.endsWith("app/theme.css")) continue;
  for (const m of readFileSync(f, "utf8").matchAll(new RegExp(`(?<![-\\w])(?:${PROPS})-([a-z][a-z0-9-]*)(?:/\\d+)?\\b`, "g"))) {
    const name = m[1];
    if (defined.has(name) || BUILTIN.test(name)) continue;
    // Utilities that are not colours at all (border-2, text-xs, ring-3, …) and
    // arbitrary values fall out here; keep only names we own or have retired.
    const OURS = /^(surface|content|edge|accent|on|success|warning|danger|info|trend|fresh|aging|stale|unavailable|observed|derived|series|ramp|delta|focus)/;
    if (!RETIRED.has(name) && !OURS.test(name)) continue;
    if (!bad.has(name)) bad.set(name, []);
    bad.get(name).push(f);
  }
}
if (bad.size === 0) console.log(`OK — every colour utility across ${files.length} files names a defined token`);
else {
  for (const [n, fs] of bad) {
    const why = RETIRED.has(n) ? "retired shadcn token" : "undefined token";
    console.log(`${why}: ${n}  ← ${[...new Set(fs)].join(", ")}`);
  }
  console.log("\nSee agent-context/lib/web-guide.md § Colour for the migration table.");
  process.exit(1);
}
