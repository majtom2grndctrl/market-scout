// Contrast and ordering checks for the palette in tokens.mjs. Exits non-zero on
// any failure so it can gate a build. Run with `pnpm theme:check`.
import { T } from "./tokens.mjs";
import { oklchToHex, lumFromOklch, hexToLum, ratio, fitGamut } from "./oklch.mjs";

// Gamut-fit every token up front so the report measures what will render.
for (const n of Object.keys(T)) for (const m of ["l", "d"]) {
  const before = T[n][m][1];
  T[n][m] = fitGamut(...T[n][m]);
  if (T[n][m][1] < before - 0.0005) console.log(`  clamp ${m} ${n}: C ${before} -> ${T[n][m][1]}`);
}

const hex = (n, m) => oklchToHex(...T[n][m]).hex;
const lum = (n, m) => lumFromOklch(...T[n][m]);
const gamut = (n, m) => oklchToHex(...T[n][m]).gamut;

let fails = 0, warns = 0;
const chk = (label, got, min, kind = "FAIL") => {
  const ok = got >= min;
  if (!ok) kind === "FAIL" ? fails++ : warns++;
  return `${ok ? "  ok  " : kind === "FAIL" ? " FAIL " : " warn "} ${label.padEnd(46)} ${got.toFixed(2).padStart(6)}  (min ${min})`;
};

for (const m of ["l", "d"]) {
  console.log(`\n================ ${m === "l" ? "LIGHT" : "DARK"} ================`);
  const out = [];
  // Gamut

  // Text ink on every surface it can land on
  const surfaces = ["surface-page", "surface-raised", "surface-sunken", "surface-overlay"];
  for (const ink of ["content-primary", "content-secondary", "content-muted", "content-link", "observed", "derived"])
    for (const s of surfaces) out.push(chk(`${ink} on ${s}`, ratio(lum(ink, m), lum(s, m)), 4.5));
  out.push(chk("content-disabled on surface-raised (exempt)", ratio(lum("content-disabled", m), lum("surface-raised", m)), 2.0, "warn"));
  out.push(chk("content-inverse on accent-solid", ratio(lum("content-inverse", m), lum("accent-solid", m)), 4.5));

  // Ink readable on its own subtle background AND on the plain page
  for (const f of ["success", "warning", "danger", "info", "trend-up", "trend-down", "trend-flat", "fresh", "aging", "stale", "unavailable"]) {
    out.push(chk(`${f}-ink on ${f}-subtle`, ratio(lum(`${f}-ink`, m), lum(`${f}-subtle`, m)), 4.5));
    out.push(chk(`${f}-ink on surface-raised`, ratio(lum(`${f}-ink`, m), lum("surface-raised", m)), 4.5));
  }
  // on-solid readable on every solid fill
  for (const f of ["accent", "success", "warning", "danger", "info", "trend-up", "trend-down", "trend-flat"])
    out.push(chk(`on-solid on ${f}-solid`, ratio(lum("on-solid", m), lum(`${f}-solid`, m)), 4.5));
  // Solid fills as chart marks: >= 3:1 vs surface
  for (const f of ["success", "warning", "danger", "info", "trend-up", "trend-down", "trend-flat"])
    out.push(chk(`${f}-solid mark on surface-raised`, ratio(lum(`${f}-solid`, m), lum("surface-raised", m)), 3.0));
  // Non-text UI: borders and focus need 3:1
  for (const s of ["surface-raised", "surface-page", "surface-sunken"]) {
    out.push(chk(`edge-strong on ${s}`, ratio(lum("edge-strong", m), lum(s, m)), 3.0));
    out.push(chk(`focus on ${s}`, ratio(lum("focus", m), lum(s, m)), 3.0));
  }
  out.push(chk("series-other mark on surface-raised", ratio(lum("series-other", m), lum("surface-raised", m)), 3.0));
  // Row states must be distinguishable from the surface they sit on
  for (const r of ["surface-row-hover", "surface-row-focus", "surface-row-selected"]) {
    const d = Math.abs(T[r][m][0] - T["surface-raised"][m][0]);
    out.push(chk(`${r} ΔL vs surface-raised`, d, 0.02, "warn"));
  }
  // Freshness must read as an ordered decay
  const cs = ["fresh", "aging", "stale", "unavailable"].map((f) => T[`${f}-ink`][m][1]);
  out.push(chk("freshness chroma strictly decays", cs.every((c, i) => i === 0 || c < cs[i - 1]) ? 1 : 0, 1));

  console.log(out.filter((l) => !/^  ok  /.test(l)).join("\n") || "  (every check passed)");
  console.log(`  ${out.filter((l) => /^  ok  /.test(l)).length} checks passed`);
}
console.log(`\nFAIL ${fails} · warn ${warns}`);
if (fails > 0) process.exit(1);
