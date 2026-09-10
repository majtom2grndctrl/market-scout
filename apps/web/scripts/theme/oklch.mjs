// OKLCH -> sRGB, plus WCAG contrast and gamut fitting. No dependencies: this
// runs in `pnpm theme:check`, which must not need an install to be useful.
export function oklchToRgb(L, C, H) {
  const h = (H * Math.PI) / 180;
  const a = C * Math.cos(h);
  const b = C * Math.sin(h);
  const l_ = L + 0.3963377774 * a + 0.2158037573 * b;
  const m_ = L - 0.1055613458 * a - 0.0638541728 * b;
  const s_ = L - 0.0894841775 * a - 1.2914855480 * b;
  const l = l_ ** 3, m = m_ ** 3, s = s_ ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.7076147010 * s,
  ];
}
const enc = (v) => (v <= 0.0031308 ? 12.92 * v : 1.055 * Math.pow(v, 1 / 2.4) - 0.055);
const clamp = (v) => Math.max(0, Math.min(1, v));
export function oklchToHex(L, C, H) {
  const lin = oklchToRgb(L, C, H);
  const gamut = lin.every((v) => v >= -0.001 && v <= 1.001);
  const hex = "#" + lin.map((v) => Math.round(clamp(enc(clamp(v))) * 255).toString(16).padStart(2, "0")).join("");
  return { hex, gamut };
}
export function lumFromOklch(L, C, H) {
  const [r, g, b] = oklchToRgb(L, C, H).map(clamp);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
export function hexToLum(hex) {
  const n = hex.replace("#", "");
  const ch = [0, 2, 4].map((i) => parseInt(n.slice(i, i + 2), 16) / 255);
  const lin = ch.map((v) => (v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)));
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2];
}
export function ratio(lumA, lumB) {
  const [hi, lo] = lumA > lumB ? [lumA, lumB] : [lumB, lumA];
  return (hi + 0.05) / (lo + 0.05);
}

// Reduce chroma (holding L and H) until the colour sits inside sRGB. Browsers
// gamut-map oklch() themselves; doing it here means the values we validate are
// the values that render.
export function fitGamut(L, C, H) {
  if (oklchToHex(L, C, H).gamut) return [L, C, H];
  let lo = 0, hi = C;
  for (let i = 0; i < 40; i++) {
    const mid = (lo + hi) / 2;
    if (oklchToHex(L, mid, H).gamut) lo = mid; else hi = mid;
  }
  return [L, Math.floor(lo * 1000) / 1000, H];
}
