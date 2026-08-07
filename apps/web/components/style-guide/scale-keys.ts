// Reads a scale back out of a generated class map, so every list in the style
// guide grows when a token is added and shrinks when one is removed.
//
// Object keys stringify. A numeric scale read back from a map arrives as "400",
// and "400" misses a lookup on a map keyed by the number 400 — the class comes
// back undefined and the specimen silently renders unstyled. numericScale
// converts back before the value ever reaches a prop or a lookup.

export function numericScale<Value extends number>(map: Record<Value, string>): Value[] {
  return Object.keys(map)
    .map(Number)
    .sort((a, b) => a - b) as Value[];
}

export function namedScale<Value extends string>(map: Record<Value, string>): Value[] {
  return Object.keys(map) as Value[];
}
