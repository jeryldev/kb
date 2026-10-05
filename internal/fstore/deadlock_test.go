package fstore

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// Two kb processes renaming notes at once never each hold the lock the
// other waits for, however their names fall into lock files.
func TestConcurrentRenamesNeverDeadlock(t *testing.T) {
	a := testStore(t)
	b := sibling(t, a)
	root := a.Vault().Root()
	abs := func(n string) string { return lockName(filepath.Join(root, n+".md")) }
	rel := func(n string) string { return lockName(n + ".md") }
	// pair1 {x,y}: rel-smaller has abs L1, other L2. pair2 {p,q}: rel-smaller has abs L2, other L1.
	var x, y, p, q string
	names := []string{}
	for i := 0; i < 20000; i++ {
		names = append(names, fmt.Sprintf("n%d", i))
	}
	byAbs := map[string][]string{}
	for _, n := range names {
		byAbs[abs(n)] = append(byAbs[abs(n)], n)
	}
	found := false
	for _, n1 := range names[:300] {
		for _, n2 := range names[300:600] {
			if abs(n1) == abs(n2) || rel(n1) == rel(n2) {
				continue
			}
			lo, hi := n1, n2
			if rel(n2) < rel(n1) {
				lo, hi = n2, n1
			}
			L1, L2 := abs(lo), abs(hi)
			// want p with abs L2 and q with abs L1 and rel(p) < rel(q)
			for _, pp := range byAbs[L2] {
				for _, qq := range byAbs[L1] {
					if pp == n1 || pp == n2 || qq == n1 || qq == n2 {
						continue
					}
					if rel(pp) < rel(qq) {
						x, y, p, q = lo, hi, pp, qq
						found = true
						goto done
					}
				}
			}
		}
	}
done:
	if !found {
		t.Fatal("no names")
	}
	nx, _ := a.CreateNote(x, "", "", "")
	np, _ := a.CreateNote(p, "", "", "")
	reload(t, b)
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		for i := 0; i < 400; i++ {
			to := y
			if i%2 == 1 {
				to = x
			}
			if _, _, err := a.RenameNote(nx.ID, to); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		for i := 0; i < 400; i++ {
			to := q
			if i%2 == 1 {
				to = p
			}
			if _, _, err := b.RenameNote(np.ID, to); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	timeout := time.After(30 * time.Second)
	for _, c := range []chan struct{}{done1, done2} {
		select {
		case <-c:
		case <-timeout:
			t.Fatal("DEADLOCK: renames hung")
		}
	}
}
