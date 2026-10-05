// English helpers for copy composed from data. The profile changes, so every
// list, article, and count is decided at render time rather than written once.

const WORDS = ["no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve"];

/** Small counts read as words in a letter; larger ones stay numerals. */
export function count(n: number): string {
  return n >= 0 && n < WORDS.length ? WORDS[n]! : figure(n);
}

/** Data quantities (postings, companies) always read as numerals. */
export function figure(n: number): string {
  return n.toLocaleString("en-US");
}

export function plural(n: number, one: string, many = `${one}s`): string {
  return n === 1 ? one : many;
}

export function counted(n: number, one: string, many?: string): string {
  return `${count(n)} ${plural(n, one, many)}`;
}

export function capital(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

// Acronyms read letter by letter: "an SRE", "a UX Designer".
const VOWEL_SOUND_LETTERS = new Set("AEFHILMNORSX");
const CONSONANT_SOUND = /^(uni|use|usa|usu|uti|eu|one|once)/i;
const SILENT_H = /^(hour|honest|honou?r|heir)/i;

export function article(phrase: string): "a" | "an" {
  const first = phrase.trim().split(/[\s/+-]/)[0] ?? "";
  if (first.length > 1 && first === first.toUpperCase() && /^[A-Z]+$/.test(first)) {
    return VOWEL_SOUND_LETTERS.has(first[0]!) ? "an" : "a";
  }
  if (CONSONANT_SOUND.test(first)) return "a";
  if (SILENT_H.test(first)) return "an";
  return /^[aeiou]/i.test(first) ? "an" : "a";
}

export function withArticle(phrase: string): string {
  return `${article(phrase)} ${phrase}`;
}

/** "A", "A and B", "A, B, and C". */
export function joinList(items: readonly string[], conjunction = "and"): string {
  if (items.length <= 1) return items[0] ?? "";
  if (items.length === 2) return `${items[0]} ${conjunction} ${items[1]}`;
  return `${items.slice(0, -1).join(", ")}, ${conjunction} ${items.at(-1)}`;
}

/**
 * The same list split into the phrases a reader takes in, punctuation
 * attached, so each item can arrive on its own beat.
 */
export function listPhrases(items: readonly string[], end = ".", conjunction = "and"): string[] {
  if (items.length === 0) return [];
  if (items.length === 1) return [`${items[0]}${end}`];
  if (items.length === 2) return [items[0]!, `${conjunction} ${items[1]}${end}`];
  return items.map((item, i) =>
    i === items.length - 1 ? `${conjunction} ${item}${end}` : `${item},`,
  );
}

export function percent(share: number): string {
  return `${Math.round(share * 100)}%`;
}

/** Case-insensitive, order-preserving. Profiles can repeat a title. */
export function unique(items: readonly string[]): string[] {
  const seen = new Set<string>();
  return items.filter((item) => {
    const k = item.trim().toLowerCase();
    if (!k || seen.has(k)) return false;
    seen.add(k);
    return true;
  });
}
