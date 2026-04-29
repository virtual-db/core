# Delta Store Reference

Technical reference for the VirtualDB `core` delta store internals.

---

## Overview

The delta store is an in-memory store that records INSERT, UPDATE, and DELETE operations without forwarding them to the source database. It acts as a mutable overlay on top of the immutable source data provided by the driver.

On every read (`RecordsSource`), the delta is overlaid on top of source records returned by the driver. The result is a unified view that reflects all in-flight mutations while leaving the source database untouched until an explicit flush or sync operation.

---

## Record keys

Every record is identified by a deterministic string key computed from its field values:

- Take all field names in the record.
- Sort them lexicographically.
- Concatenate as `field=value|field=value`.

**Example:** a record `{"id": 1, "name": "alice", "age": 30}` produces the key:

```/dev/null/example.go#L1-1
age=30|id=1|name=alice
```

This key is the primary lookup key for all delta operations. Two records are considered the same source row if and only if their record keys match.

---

## Three mutation buckets per table

Each table in the delta store maintains three independent buckets.

| Bucket | Key type | Contents |
|---|---|---|
| `inserts` | Current record key | Net-new rows with no counterpart in the source database. |
| `updates` | Stable source key | Overlay records for existing source rows. Replaces the source row on read. |
| `tombstones` | Stable source key | Deletion markers. Suppresses the corresponding source row on read. |

The distinction between the current record key (used for `inserts`) and the stable source key (used for `updates` and `tombstones`) is important: when a row is updated, its field values — and therefore its record key — change. The stable source key anchors all mutations for a given row back to the key the row had when it first entered the delta.

---

## Stable keys and `currentToStable` tracking

When a row is updated, its record key changes. The delta store maintains a `currentToStable` map:

```/dev/null/example.go#L1-1
RecordKey(after_update) → RecordKey(original_source_row)
```

This allows subsequent updates and deletes on the same row to trace back to the correct `updates` or `tombstones` slot, regardless of how many times the row has been mutated.

### Example: three operations on the same row

Assume the source row originally has key `id=1|name=alice`.

**First UPDATE** — changes `name` to `bob`:
- Stored in `updates["id=1|name=alice"]`.
- `currentToStable["id=1|name=bob"] = "id=1|name=alice"`.

**Second UPDATE** — changes `name` to `carol`:
- Looks up `currentToStable["id=1|name=bob"]` → finds `"id=1|name=alice"`.
- Overwrites `updates["id=1|name=alice"]` with the new record.
- Adds `currentToStable["id=1|name=carol"] = "id=1|name=alice"`.
- Removes the now-stale `currentToStable["id=1|name=bob"]` entry.

**DELETE**:
- Looks up `currentToStable["id=1|name=carol"]` → finds `"id=1|name=alice"`.
- Removes `updates["id=1|name=alice"]`.
- Adds `tombstones["id=1|name=alice"]`.

At no point does the original stable key `id=1|name=alice` change, which keeps the `updates` and `tombstones` buckets consistent across the entire mutation chain.

---

## `TxDelta` and transaction isolation

When a connection begins a transaction (`TransactionBegun`), it receives a fresh private `delta.Delta` called `TxDelta`. All writes within the transaction — INSERT, UPDATE, and DELETE — go to `TxDelta` rather than the live delta. This makes the writes invisible to all other connections for the duration of the transaction.

### On COMMIT (`TransactionCommitted`)

`TxDelta` is merged into the live delta using last-write-wins semantics:

- **Inserts** from `TxDelta` are added to the live delta's `inserts` bucket.
- **Updates** from `TxDelta` overwrite any existing live delta update for the same stable key.
- **Tombstones** from `TxDelta` are applied to the live delta's `tombstones` bucket. Any existing update for the same stable key is removed.

### On ROLLBACK (`TransactionRolledBack`)

`TxDelta` is discarded entirely. The live delta is never touched during an in-progress transaction, so rollback requires no undo work — there is nothing to reverse.

---

## Cross-boundary stable key resolution

In autocommit mode (no explicit transaction), each `DriverAPI` write call operates without a `TxDelta`. The `ApplyUpdateWithFallback` and `ApplyDeleteWithFallback` functions handle stable key resolution across the boundary between a fresh write context and the live delta.

When resolving a stable key for an UPDATE or DELETE and no entry is found in the immediate context's `currentToStable` map, these functions fall through to the live delta's `currentToStable` map. This ensures that a chain of autocommit writes on the same row resolves correctly even though each write arrives in an independent call.

---

## The overlay algorithm — `RecordsSource`

A two-pass overlay is applied every time the driver calls `RecordsSource`.

### Pass 1 — apply the live delta over source records

1. For each source record returned by the driver, compute its record key.
2. Check `currentToStable` for the computed key. If an entry exists, use the stable key for all subsequent lookups; otherwise use the computed key directly.
3. If the stable key has a tombstone in the live delta → suppress the row (exclude it from the result).
4. If the stable key has an entry in the live delta `updates` bucket → replace the source row with the overlay record.
5. If the stable key has neither a tombstone nor an update, pass the source row through unchanged.
6. After all source rows are processed, iterate the live delta `inserts` bucket. Append any insert whose key is not already represented in the result set.

The output of Pass 1 is the merged view of the source data and the live delta.

### Pass 2 — apply `TxDelta` (read-your-own-writes)

If the connection has an open `TxDelta`, repeat the identical overlay algorithm on top of the Pass 1 result, using `TxDelta` as the delta source instead of the live delta.

This implements **read-your-own-writes**: uncommitted changes made within the current transaction are visible to reads within the same transaction, but are not visible to any other connection until the transaction commits.

---

## `SchemaLoaded` and stable key resolution

```/dev/null/example.go#L1-1
DriverAPI.SchemaLoaded(table string, columns []string, pkCol string)
```

Calling `SchemaLoaded` populates the schema cache with the column list and the primary key column name for a given table. When `pkCol` is set, the delta store uses it as a hint for stable key construction.

When a primary key column is known, stable keys can be derived from the primary key field alone rather than from all fields in the record. This significantly reduces the chance of mismatches caused by unrelated field changes (for example, a `last_modified` timestamp that changes on every write).

If `pkCol` is empty or `SchemaLoaded` has not been called for a table, the delta store falls back to the full lexicographic record key described above.

---

## Concurrency

| Resource | Mechanism |
|---|---|
| Live `delta.Delta` | Protected by a `sync.RWMutex`. |
| Read operations (overlay, `TableState`) | Acquire a read lock (`RLock`). Multiple concurrent readers are permitted. |
| Write operations (INSERT, UPDATE, DELETE, commit merge) | Acquire a write lock (`Lock`). Exclusive access. |
| `TxDelta` | Per-connection. Never shared between goroutines. No locking required. |

Handlers that read from `hctx.Global` do not need to acquire any lock; the `GlobalContext` is immutable after the `seal` point and is safe for concurrent access without synchronisation.

---

## `DeltaTableState`

`delta.TableState(table string)` returns a full copy of a table's mutation state as a `DeltaTableState` struct:

```/dev/null/example.go#L1-6
type DeltaTableState struct {
    Inserts    map[string]map[string]any
    Updates    map[string]map[string]any
    Tombstones map[string]struct{}
}
```

Each map key is a record key string as described in [Record keys](#record-keys). The values in `Inserts` and `Updates` are copies of the stored records; mutations to the returned maps do not affect the live delta.

`TableState` acquires a read lock for the duration of the copy. It is safe to call concurrently with other read operations.

Common uses:
- Inspecting delta contents during debugging.
- Building read-side projections or materialised views outside the normal `RecordsSource` overlay path.
- Snapshotting delta state before a destructive operation.