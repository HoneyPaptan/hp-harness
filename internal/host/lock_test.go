package host

import (
	"path/filepath"
	"testing"
)

func TestMemoryLock(t *testing.T) {
	m := NewMemory()
	unlock, err := m.Lock("/hp/runs/recover.lock")
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Lock("/hp/runs/recover.lock")
	if err == nil {
		t.Fatal("second lock acquired")
	}
	unlock()
	unlock2, err := m.Lock("/hp/runs/recover.lock")
	if err != nil {
		t.Fatalf("relock after unlock: %v", err)
	}
	unlock2()
}

func TestNativeLock(t *testing.T) {
	n := Native{}
	lock := filepath.Join(t.TempDir(), "lock")
	unlock, err := n.Lock(lock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.Lock(lock)
	if err == nil {
		t.Fatal("second flock succeeded")
	}
	unlock()
	unlock2, err := n.Lock(lock)
	if err != nil {
		t.Fatalf("relock after unlock: %v", err)
	}
	unlock2()
}
