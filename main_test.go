package main

import "testing"

func assertName(t *testing.T, got *User, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Fatalf("want not found, got %+v", got)
		}
		return
	}
	if got == nil || got.Name != want {
		t.Fatalf("want %q, got %+v", want, got)
	}
}

func TestDirtyReadPrevented(t *testing.T) {
	for _, level := range []IsolationLevel{ReadCommitted, RepeatableRead} {
		db := NewDB()
		t1 := db.Begin(level)
		t1.Insert(1, "bob", "0991")

		t2 := db.Begin(level)
		assertName(t, t2.Get(1), "") // t1 not commited
	}
}

func TestNonRepeatableRead(t *testing.T) {
	setup := func(level IsolationLevel) (*DB, *Tx) {
		db := NewDB()
		t1 := db.Begin(level)
		t1.Insert(1, "bob", "0991")
		t1.Commit()
		return db, db.Begin(level)
	}

	t.Run("repeatable read keeps the old value", func(t *testing.T) {
		db, t2 := setup(RepeatableRead)
		assertName(t, t2.Get(1), "bob")

		t3 := db.Begin(RepeatableRead)
		if err := t3.Update(1, "john", "0991"); err != nil {
			t.Fatal(err)
		}
		t3.Commit()

		assertName(t, t2.Get(1), "bob")
	})

	t.Run("read committed sees the new value", func(t *testing.T) {
		db, t2 := setup(ReadCommitted)
		assertName(t, t2.Get(1), "bob")

		t3 := db.Begin(ReadCommitted)
		if err := t3.Update(1, "john", "0991"); err != nil {
			t.Fatal(err)
		}
		t3.Commit()

		assertName(t, t2.Get(1), "john")
	})

	t.Run("repeatable read fails on conflict", func(t *testing.T) {
		db, t2 := setup(RepeatableRead)

		assertName(t, t2.Get(1), "bob")
		t3 := db.Begin(RepeatableRead)
		if err := t3.Update(1, "john", "0991"); err != nil {
			t.Fatal(err)
		}
		if err := t2.Update(1, "ahmad", "0991"); err == nil {
			t.Fatal("expected to fail on conflict for repeatable read updates")
		}
		t3.Commit()
		t2.Commit()
	})
}

func TestInsertNewRecord(t *testing.T) {
	db := NewDB()

	tx := db.Begin(ReadCommitted)
	tx.Insert(1, "bob", "0991")
	tx.Commit()

	if len(db.users[1]) != 1 {
		t.Fatalf("expect to create a new user")
	}

	if db.users[1][0].Xmax != 0 {
		t.Fatalf("expect that new tuple created have Xmax 0")
	}
}
