// The table's column template, shared by the header, every row, and each row's
// detail so all three align to one grid.

/** Rank, role, builds on, you bring, also asks for, postings, companies, pin. Wide containers only. */
export const COLS =
  "@5xl:grid-cols-[3.25rem_minmax(0,2.15fr)_minmax(0,0.95fr)_minmax(0,1.05fr)_minmax(0,2.2fr)_4.25rem_5rem_2.5rem]";

/**
 * Below the wide breakpoint: rank, reasons, pin. Mid-width containers pair
 * the reasons two-up; the narrowest stack them in one column.
 */
export const NARROW = "grid-cols-[2.5rem_minmax(0,1fr)_3rem] @2xl:grid-cols-[2.5rem_minmax(0,1fr)_minmax(0,1fr)_3rem]";
