# PostgreSQL MVCC Simulation in Go

An educational, in-memory implementation of PostgreSQL's **Multi-Version Concurrency Control (MVCC)** mechanism written in Go.

This project demonstrates how relational databases like PostgreSQL manage concurrent transactions without locking readers, implementing tuple versioning (`xmin` / `xmax`), database snapshots, transaction isolation levels, and write-conflict detection.

> [!NOTE]
> **Reference Video**: For a detailed conceptual walkthrough of how PostgreSQL implements MVCC and isolation levels, see this [YouTube Video](https://youtu.be/JV8m1ZvlBcM?si=9MKmmSDO_KJI3Kkt).

---

## Features

- **Tuple Versioning (`xmin` / `xmax`)**: Instead of mutating rows in place, updates and deletes append new tuple versions to version chains.
- **Transaction Isolation Levels**:
  - `Read Committed`: Takes a new snapshot before each operation, observing newly committed data from other transactions.
  - `Repeatable Read`: Reuses the snapshot captured at transaction start (`Begin`), guaranteeing consistent reads throughout the transaction.
- **Snapshot-Based Visibility Rules**: Accurate determination of whether a given tuple version is visible to a running transaction based on transaction start times, active transactions, and commit statuses.
- **Write Conflict Detection**: Rejects updates or deletes (`ErrConflict`) on rows already modified by another active or committed transaction in `Repeatable Read` mode.
- **Comprehensive Unit Tests**: Validates prevention of dirty reads, non-repeatable reads, conflict handling, and version creation.

---

## How It Works

### 1. Tuple Versioning

Each record version is stored as a `Tuple` in an in-memory version chain:

```go
type Tuple struct {
    Xmin int  // Transaction ID that created this version
    Xmax int  // Transaction ID that deleted or replaced this version (0 = alive)
    Data User // The payload data (ID, Name, Phone)
}
```

- **Insert**: Appends a new tuple with `Xmin = currentTxID` and `Xmax = 0`.
- **Update**: Marks the current visible tuple's `Xmax = currentTxID`, then appends a new tuple with `Xmin = currentTxID` and `Xmax = 0`.
- **Delete**: Sets the visible tuple's `Xmax = currentTxID`.

#### Example Version Chain (Row ID: 1)

```text
Initial Insert (Tx 1):
  [xmin: 1, xmax: 0, Data: "bob"]

After Update to "john" (Tx 2):
  [xmin: 1, xmax: 2, Data: "bob"]  --> (old version, replaced by Tx 2)
  [xmin: 2, xmax: 0, Data: "john"] --> (current version)
```

---

### 2. Snapshots & Visibility Rules

A `Snapshot` captures the database state from the perspective of a transaction:

```go
type Snapshot struct {
    Xmax   int          // Next TxID at snapshot time (any TxID >= Xmax started later)
    Active map[int]bool // TxIDs that were in-progress when the snapshot was taken
}
```

A transaction decides whether it can see a transaction `id` (`sees`):
1. **Self**: A transaction always sees its own changes (`id == tx.ID`).
2. **Future Transactions**: If `id >= snapshot.Xmax`, the transaction started after the snapshot; hence it is **invisible**.
3. **In-Flight Transactions**: If `snapshot.Active[id]` is true, the transaction was still running when the snapshot was taken; hence it is **invisible**.
4. **Committed Status**: If the transaction finished before the snapshot, it is visible **only if** `txStatus[id] == Committed`.

A tuple version is visible to a transaction (`visible`) if:
1. The creating transaction (`Xmin`) is visible to the transaction.
2. The tuple has either never been deleted (`Xmax == 0`), or the deleting transaction (`Xmax`) is **not** visible to this transaction.

---

### 3. Isolation Levels

| Isolation Level | Snapshot Policy | Behavior |
| :--- | :--- | :--- |
| **Read Committed** | Refreshed on every operation (`Get`, `Update`, `Delete`) | Can see changes committed by other transactions between statements (non-repeatable reads possible). |
| **Repeatable Read** | Captured once at `Begin()` and preserved | Guarantees identical read results across multiple reads within the same transaction. |

---

## Project Structure

```text
.
├── go.mod        # Go module definition (Go 1.22+)
├── main.go       # Core MVCC engine (DB, Tx, Tuple, Snapshot, Visibility logic)
├── main_test.go  # Unit test suite verifying isolation and conflict semantics
└── README.md     # Documentation
```

---

## Usage Example

```go
package main

import (
    "fmt"
)

func main() {
    db := NewDB()

    // Transaction 1: Insert user
    t1 := db.Begin(ReadCommitted)
    t1.Insert(1, "Alice", "0912-000-0000")
    t1.Commit()

    // Transaction 2: Read under Repeatable Read
    t2 := db.Begin(RepeatableRead)
    user := t2.Get(1)
    fmt.Printf("Tx2 read 1: %s\n", user.Name) // Alice

    // Transaction 3: Update user concurrently
    t3 := db.Begin(ReadCommitted)
    _ = t3.Update(1, "Alice Smith", "0912-000-0000")
    t3.Commit()

    // Transaction 2 reads again: Repeatable Read still sees original snapshot
    userRepeat := t2.Get(1)
    fmt.Printf("Tx2 read 2: %s\n", userRepeat.Name) // Alice

    // Transaction 4 reads under Read Committed: sees latest committed update
    t4 := db.Begin(ReadCommitted)
    fmt.Printf("Tx4 read: %s\n", t4.Get(1).Name) // Alice Smith
}
```

---

## Running Tests

Run the test suite using `go test`:

```bash
go test -v ./...
```

### Test Coverage Highlights

- `TestDirtyReadPrevented`: Verifies that uncommitted inserts are hidden from other concurrent transactions.
- `TestNonRepeatableRead`:
  - Validates that `RepeatableRead` keeps the initial value despite concurrent commits.
  - Validates that `ReadCommitted` picks up concurrent commits on subsequent reads.
  - Validates that concurrent updates on the same row trigger a write conflict (`ErrConflict`).
- `TestInsertNewRecord`: Verifies tuple creation and initial state (`Xmax == 0`).

---

## Current Scope & Future Enhancements

This project is a teaching and exploration vehicle. Potential extensions include:
- **Vacuuming / Garbage Collection**: Reclaiming old dead tuple versions where `Xmax` is older than all active snapshots.
- **Aborted Transaction Rollback**: Explicit reclamation or skipping of aborted tuples.
- **Thread Safety**: Adding sync primitives (`sync.RWMutex`) to support concurrent goroutines.
- **Serializable Isolation**: Implementing SSI (Serializable Snapshot Isolation) with read/write dependency tracking (`siREAD` locks).

---

## References

- [YouTube: PostgreSQL MVCC & Transaction Isolation Walkthrough](https://youtu.be/JV8m1ZvlBcM?si=9MKmmSDO_KJI3Kkt)
