# navdep

Status as of **2026-09-24 01:13 +03:00**.

Interactive console for a folder of Navision object text files. One process, no external dependencies. `build` indexes the folder and builds a reverse map of compile-time references. `dependents` reads that map.

## Run

Requires Go 1.23.

```
go run ./cmd/navdep
```

```
navdep> build "C:\NAV\Objects"
objects: 2, links: 2, unresolved: 0
navdep> dependents c12
c11 - Gen. Jnl.-Check Line
t81 - Gen. Journal Line
navdep> help
navdep> exit
```

A bad command prints `error:` and the session continues. `exit` and `quit` end it. A folder path may be double-quoted.

## What build does

`build <folder>` scans the top level of the folder and replaces the in-memory index and reverse map. A failed build leaves the previous map in place.

Accepted files are `*.txt` named `{prefix}{id} - {name}.txt`, for example `c50001 - Exchange Management.txt`. The name is the text after the first hyphen, so it may contain hyphens. Every other file is ignored.

| Prefix | OBJECT type |
| --- | --- |
| `t` | Table |
| `p` | Page |
| `r` | Report |
| `c` | Codeunit |
| `x` | XMLport |
| `q` | Query |
| `m` | MenuSuite |
| `f` | Form |
| `d` | Dataport |

The `OBJECT` header supplies the name used for lookup, either bare (`OBJECT Codeunit 50001 Exchange Management`) or quoted (`OBJECT Codeunit 12 "Gen. Jnl.-Post Line"`). The header type and id must match the file name. A mismatch, a missing header, or a second file with the same key is skipped and printed as `warning:`. The first file in name order is kept when the key is duplicated.

Names are matched without regard to case. `CODEUNIT::`, `DATABASE::` (tables), `PAGE::`, `REPORT::`, `XMLPORT::`, `QUERY::`, `FORM::`, and `DATAPORT::` resolve to that type. A name shared by two objects of the same type does not resolve.

File text is read as UTF-8, as UTF-16 when a BOM is present, or as Windows-1252 when the bytes are not valid UTF-8.

After indexing headers, each accepted file is scanned for compile-time references. Comments, single-quoted strings, and TableData permissions are not references. A numeric reference such as `Codeunit 12` becomes a key even when that file is missing. An object is not listed as a dependent of itself. Each caller is stored once.

The summary line is `objects`, `links`, and `unresolved`. Object count is the number of accepted files. Link count is the number of unique caller edges. Unresolved count is names that do not resolve (unknown, or the same name used by two objects of the same type).

## What dependents does

`dependents <key>` requires a successful `build` first. The key is a type prefix plus an id, such as `c12` or `t81`, and the match ignores case.

Each caller is printed on its own line as `key - name`, sorted by type prefix then numeric id:

```
c11 - Gen. Jnl.-Check Line
t81 - Gen. Journal Line
```
