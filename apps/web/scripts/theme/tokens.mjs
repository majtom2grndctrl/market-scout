/**
 * The colour palette, as OKLCH [L, C, H] per mode. This file is the source of
 * truth; `app/theme.css` is generated from it by `pnpm theme:build`.
 *
 * Two invariants hold across the whole set and are checked by `pnpm theme:check`:
 *   - `on-solid` clears 4.5:1 on every `*-solid`, so light solids stay at or
 *     below L 0.52 and dark solids at or above L 0.68.
 *   - every `*-ink` clears 4.5:1 on its own `*-subtle` AND on `surface-raised`.
 *
 * Chroma is fitted to sRGB at generation time, so a value here may render at a
 * lower chroma than written; the check reports every clamp.
 */
export const SERIES_LIGHT = ["#1baf7a","#eda100","#e87ba4","#008300","#4a3aa7","#e34948","#2a78d6","#eb6834"];
export const SERIES_DARK  = ["#199e70","#c98500","#d55181","#008300","#9085e9","#e66767","#3987e5","#d95926"];
export const RAMP = ["#cde2fb","#9ec5f4","#6da7ec","#3987e5","#256abf","#184f95","#0d366b"];

export const T = {
  // ---- Surface -------------------------------------------------------------
  "surface-page":         { l: [0.985, 0, 0],      d: [0.190, 0, 0] },
  "surface-raised":       { l: [1.000, 0, 0],      d: [0.235, 0, 0] },
  "surface-sunken":       { l: [0.955, 0, 0],      d: [0.155, 0, 0] },
  "surface-overlay":      { l: [1.000, 0, 0],      d: [0.265, 0, 0] },
  "surface-row-hover":    { l: [0.968, 0, 0],      d: [0.275, 0, 0] },
  "surface-row-focus":    { l: [0.950, 0, 0],      d: [0.305, 0, 0] },
  "surface-row-selected": { l: [0.925, 0, 0],      d: [0.345, 0, 0] },
  // ---- Content -------------------------------------------------------------
  "content-primary":      { l: [0.160, 0, 0],      d: [0.970, 0, 0] },
  "content-secondary":    { l: [0.420, 0, 0],      d: [0.780, 0, 0] },
  "content-muted":        { l: [0.530, 0, 0],      d: [0.680, 0, 0] },
  "content-disabled":     { l: [0.720, 0, 0],      d: [0.480, 0, 0] },
  "content-inverse":      { l: [0.985, 0, 0],      d: [0.160, 0, 0] },
  "content-link":         { l: [0.420, 0.06, 265], d: [0.800, 0.07, 265] },
  // ---- Edge ----------------------------------------------------------------
  "edge-hairline":        { l: [0.935, 0, 0],      d: [0.300, 0, 0] },
  "edge":                 { l: [0.895, 0, 0],      d: [0.360, 0, 0] },
  "edge-strong":          { l: [0.620, 0, 0],      d: [0.520, 0, 0] },
  "focus":                { l: [0.450, 0, 0],      d: [0.800, 0, 0] },
  // ---- Accent (achromatic ink) ---------------------------------------------
  "accent-solid":         { l: [0.220, 0, 0],      d: [0.920, 0, 0] },
  "accent-hover":         { l: [0.320, 0, 0],      d: [0.980, 0, 0] },
  "accent-active":        { l: [0.140, 0, 0],      d: [0.840, 0, 0] },
  "accent-subtle":        { l: [0.945, 0, 0],      d: [0.310, 0, 0] },
  "on-solid":             { l: [0.990, 0, 0],      d: [0.180, 0, 0] },
  // ---- Status --------------------------------------------------------------
  "success-solid":  { l: [0.505, 0.135, 150], d: [0.740, 0.155, 150] },
  "success-subtle": { l: [0.958, 0.030, 150], d: [0.290, 0.045, 150] },
  "success-edge":   { l: [0.870, 0.062, 150], d: [0.430, 0.080, 150] },
  "success-ink":    { l: [0.450, 0.120, 150], d: [0.810, 0.140, 150] },
  "warning-solid":  { l: [0.520, 0.125,  85], d: [0.800, 0.150,  85] },
  "warning-subtle": { l: [0.962, 0.042,  85], d: [0.300, 0.050,  85] },
  "warning-edge":   { l: [0.875, 0.085,  85], d: [0.445, 0.090,  85] },
  "warning-ink":    { l: [0.460, 0.110,  85], d: [0.860, 0.145,  85] },
  "danger-solid":   { l: [0.505, 0.195,  27], d: [0.700, 0.200,  27] },
  "danger-subtle":  { l: [0.958, 0.030,  27], d: [0.300, 0.055,  27] },
  "danger-edge":    { l: [0.870, 0.065,  27], d: [0.445, 0.100,  27] },
  "danger-ink":     { l: [0.470, 0.185,  27], d: [0.780, 0.170,  27] },
  "info-solid":     { l: [0.480, 0.175, 295], d: [0.720, 0.185, 295] },
  "info-subtle":    { l: [0.958, 0.030, 295], d: [0.305, 0.060, 295] },
  "info-edge":      { l: [0.875, 0.060, 295], d: [0.450, 0.100, 295] },
  "info-ink":       { l: [0.455, 0.170, 295], d: [0.820, 0.150, 295] },
  // ---- Trend (direction, never status) -------------------------------------
  "trend-up-solid":    { l: [0.505, 0.165, 255], d: [0.715, 0.170, 255] },
  "trend-up-subtle":   { l: [0.958, 0.030, 255], d: [0.300, 0.055, 255] },
  "trend-up-edge":     { l: [0.870, 0.065, 255], d: [0.445, 0.095, 255] },
  "trend-up-ink":      { l: [0.460, 0.160, 255], d: [0.800, 0.150, 255] },
  "trend-down-solid":  { l: [0.520, 0.155,  45], d: [0.760, 0.160,  45] },
  "trend-down-subtle": { l: [0.960, 0.035,  45], d: [0.300, 0.050,  45] },
  "trend-down-edge":   { l: [0.875, 0.075,  45], d: [0.445, 0.090,  45] },
  "trend-down-ink":    { l: [0.470, 0.140,  45], d: [0.830, 0.140,  45] },
  "trend-flat-solid":  { l: [0.550, 0, 0],       d: [0.700, 0, 0] },
  "trend-flat-subtle": { l: [0.955, 0, 0],       d: [0.300, 0, 0] },
  "trend-flat-edge":   { l: [0.870, 0, 0],       d: [0.445, 0, 0] },
  "trend-flat-ink":    { l: [0.460, 0, 0],       d: [0.760, 0, 0] },
  // ---- Freshness (one hue, chroma decays with age) -------------------------
  "fresh-ink":          { l: [0.470, 0.105, 195], d: [0.790, 0.100, 195] },
  "fresh-subtle":       { l: [0.955, 0.035, 195], d: [0.295, 0.050, 195] },
  "fresh-edge":         { l: [0.865, 0.070, 195], d: [0.440, 0.085, 195] },
  "aging-ink":          { l: [0.495, 0.045, 195], d: [0.765, 0.048, 195] },
  "aging-subtle":       { l: [0.955, 0.019, 195], d: [0.290, 0.026, 195] },
  "aging-edge":         { l: [0.868, 0.038, 195], d: [0.435, 0.046, 195] },
  "stale-ink":          { l: [0.520, 0.016, 195], d: [0.730, 0.020, 195] },
  "stale-subtle":       { l: [0.958, 0.007, 195], d: [0.285, 0.010, 195] },
  "stale-edge":         { l: [0.870, 0.014, 195], d: [0.430, 0.018, 195] },
  "unavailable-ink":    { l: [0.520, 0, 0],       d: [0.700, 0, 0] },
  "unavailable-subtle": { l: [0.958, 0, 0],       d: [0.280, 0, 0] },
  "unavailable-edge":   { l: [0.870, 0, 0],       d: [0.425, 0, 0] },
  // ---- Provenance ----------------------------------------------------------
  "observed":        { l: [0.160, 0, 0], d: [0.970, 0, 0] },
  "derived":         { l: [0.500, 0, 0], d: [0.720, 0, 0] },
  "derived-subtle":  { l: [0.965, 0, 0], d: [0.285, 0, 0] },
  // ---- Categorical overflow ------------------------------------------------
  "series-other":    { l: [0.600, 0, 0], d: [0.620, 0, 0] },
  // ---- Diverging (delta) ---------------------------------------------------
  "delta-up-3":   { l: [0.520, 0.170, 255], d: [0.760, 0.140, 255] },
  "delta-up-2":   { l: [0.650, 0.130, 255], d: [0.620, 0.150, 255] },
  "delta-up-1":   { l: [0.800, 0.065, 255], d: [0.480, 0.100, 255] },
  "delta-mid":    { l: [0.930, 0, 0],       d: [0.320, 0, 0] },
  "delta-down-1": { l: [0.800, 0.065,  45], d: [0.480, 0.090,  45] },
  "delta-down-2": { l: [0.650, 0.135,  45], d: [0.620, 0.140,  45] },
  "delta-down-3": { l: [0.520, 0.155,  45], d: [0.760, 0.130,  45] },
};
