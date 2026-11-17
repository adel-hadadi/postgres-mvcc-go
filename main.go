package main

import (
	"errors"
	"slices"
)

import "fmt"

type TxStatus int

const (
	InProgress TxStatus = iota
	Committed
	Aborted
)

type IsolationLevel int

const (
	ReadCommitted IsolationLevel = iota
	RepeatableRead
)

var ErrConflict = errors.New("write conflict: row changed by another transaction")

type (
	Snapshot struct {
		Xmax   int          // next txid at snapshot time: anything >= Xmax started later
		Active map[int]bool // Tx ids that were in progress at start time
	}

	User struct {
		ID    int
		Name  string
		Phone string
	}

	Tuple struct {
		Xmin int // txid that created this version
		Xmax int // txid that deleted/replaced it, 0 = not delete
		Data User
	}

	Tx struct {
		ID    int
		db    *DB
		Level IsolationLevel
		Snap  *Snapshot
	}

	DB struct {
		users    map[int][]*Tuple
		txStatus map[int]TxStatus
		nextTxID int
	}
)

func NewDB() *DB {
	return &DB{
		users:    make(map[int][]*Tuple),
		txStatus: make(map[int]TxStatus),
		nextTxID: 1,
	}
}

func (d *DB) takeSnapshot() *Snapshot {
	s := &Snapshot{Xmax: d.nextTxID, Active: map[int]bool{}}
	for id, st := range d.txStatus {
		if st == InProgress {
			s.Active[id] = true
		}
	}
	return s
}

func (d *DB) newTxID() int {
	id := d.nextTxID
	d.nextTxID++
	return id
}

func (d *DB) Begin(level IsolationLevel) *Tx {
	id := d.newTxID()
	d.txStatus[id] = InProgress
	return &Tx{
		ID:    id,
		db:    d,
		Level: level,
		Snap:  d.takeSnapshot(),
	}
}

func (tx *Tx) Commit() {
	tx.db.txStatus[tx.ID] = Committed
}

func (tx *Tx) Abort() {
	tx.db.txStatus[tx.ID] = Aborted
}

func (tx *Tx) Insert(id int, name, phone string) {
	tx.db.users[id] = append(tx.db.users[id], &Tuple{
		Xmin: tx.ID,
		Data: User{
			ID:    id,
			Name:  name,
			Phone: phone,
		},
	})
}

func (tx *Tx) Update(id int, name, phone string) error {
	if tx.Level == ReadCommitted {
		tx.Snap = tx.db.takeSnapshot()
	}

	old := tx.db.findVisible(tx, id)
	if old == nil {
		return fmt.Errorf("user %d not found", id)
	}
	if old.Xmax != 0 && tx.db.txStatus[old.Xmax] != Aborted {
		return ErrConflict
	}
	old.Xmax = tx.ID

	tx.db.users[id] = append(tx.db.users[id], &Tuple{
		Xmin: tx.ID,
		Data: User{
			ID:    id,
			Name:  name,
			Phone: phone,
		},
	})

	return nil
}

func (tx *Tx) Delete(id int) error {
	if tx.Level == ReadCommitted {
		tx.Snap = tx.db.takeSnapshot()
	}

	old := tx.db.findVisible(tx, id)
	if old == nil {
		return fmt.Errorf("user %d not found", id)
	}
	if old.Xmax != 0 && tx.db.txStatus[old.Xmax] != Aborted {
		return ErrConflict
	}

	old.Xmax = tx.ID
	return nil
}

func (tx *Tx) Get(id int) *User {
	if tx.Level == ReadCommitted {
		tx.Snap = tx.db.takeSnapshot()
	}

	t := tx.db.findVisible(tx, id)
	if t == nil {
		return nil
	}

	return &t.Data
}

func (d *DB) sees(tx *Tx, id int) bool {
	if id == tx.ID {
		return true
	}
	if id >= tx.Snap.Xmax {
		return false // started after the snapshot
	}

	if tx.Snap.Active[id] {
		return false // was still running at my snapshot
	}

	return d.txStatus[id] == Committed // Finished before snapshot: committed or aborted
}

func (d *DB) visible(tx *Tx, t *Tuple) bool {
	if !d.sees(tx, t.Xmin) {
		return false // creator not visible to me
	}

	if t.Xmax == 0 {
		return true // never deleted
	}

	return !d.sees(tx, t.Xmax) // deleted only if I can see the deleted
}

func (d *DB) findVisible(tx *Tx, id int) *Tuple {
	versions := d.users[id]
	for _, version := range slices.Backward(versions) {
		if d.visible(tx, version) {
			return version
		}
	}
	return nil
}

func (d *DB) dump() {
	fmt.Println("--- users heap ---")
	for _, versions := range d.users {
		for _, t := range versions {
			fmt.Printf("xmin=%d xmax=%d %+v\n", t.Xmin, t.Xmax, t.Data)
		}
	}
}
