# Synthetic MOK dumps

Each size is a pair written by one run: `dump.sql` builds the SQLite dump the MOK app and the
importer read, and `expected.json` is the state Core should end in after importing it.

| Size | Period | Seed | Rows | Checked in the MOK app |
|---|---|---|---:|---|
| S | 2024-07-01 … 2024-12-31 | 20240701 | 413 | yes, 2026-10-14: every live balance to the cent |

**S is a temporary copy** of `fintrace-mok-importer`'s
`src/test/resources/synthetic/mok-synthetic-S-dump.*` (commit `d302064`), written by the importer's
Kotlin generator. It is replaced when `fintrace-dumpgen` writes the dumps here (`design-decisions.md`
§7.9); M and L arrive with it.
