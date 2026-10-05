// Copy helpers. Every sentence on the page is assembled from data, so the
// grammar has to hold for any title, count, or list length.

// Letters whose spoken name starts with a vowel sound: "an SRE", "a UX Designer".
const VOWEL_SOUND_LETTERS = "AEFHILMNORSX";

export function article(phrase: string): "a" | "an" {
  const first = phrase.trim().split(/\s+/)[0] ?? "";
  if (/^[A-Z]{2,}$/.test(first.replace(/[^A-Za-z]/g, ""))) {
    return VOWEL_SOUND_LETTERS.includes(first.charAt(0)) ? "an" : "a";
  }
  if (/^(uni|use|usu|eu|one\b|once)/i.test(first)) return "a";
  if (/^(hour|honest|honou?r)/i.test(first)) return "an";
  return /^[aeiou]/i.test(first) ? "an" : "a";
}

/** "a", "a and b", "a, b and c". */
export function joinList(items: readonly string[]): string {
  if (items.length <= 1) return items[0] ?? "";
  return `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

const WORDS = [
  "zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten",
  "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen", "twenty",
];

/** Spelled out through twenty, as display type reads better in words. */
export function numberWord(n: number, capitalise = false): string {
  const word = WORDS[n] ?? n.toLocaleString("en-US");
  return capitalise ? word.charAt(0).toUpperCase() + word.slice(1) : word;
}

export function plural(n: number, one: string, many: string): string {
  return n === 1 ? one : many;
}

/** share is P(skill | role); it describes the role's postings, never the person. */
export function percentOfPostings(share: number): string {
  return `${Math.round(share * 100)}%`;
}
