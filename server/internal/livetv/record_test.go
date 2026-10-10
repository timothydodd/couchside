package livetv

import (
	"context"
	"testing"

	"github.com/timothydodd/couchside/internal/db"
)

// Pressing Record on an airing someone else's rule scheduled must not make
// the presser its owner (who may then cancel it or delete the file).
func TestRecordDoesNotTakeOverOwnership(t *testing.T) {
	s, d, _ := newTestService(t)
	ctx := context.Background()
	alice, err := d.CreateProfile(ctx, "Alice", "accent")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := d.CreateProfile(ctx, "Bob", "accent")
	if err != nil {
		t.Fatal(err)
	}
	rule, _, err := s.CreateRule(ctx, firstProgram(t, d), "missing", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetRuleOwner(ctx, rule.ID, alice.ID); err != nil {
		t.Fatal(err)
	}
	progs, err := d.UpcomingSeriesPrograms(ctx, "SH123", 0)
	if err != nil {
		t.Fatal(err)
	}
	recs, _ := d.Recordings(ctx, 0, 0)
	ruleRec := map[int64]db.Recording{} // start → the rule's row
	for _, r := range recs {
		ruleRec[r.StartAt] = r
	}
	var ruled, fresh db.Program
	for _, p := range progs {
		if r, ok := ruleRec[p.StartAt]; ok && r.Channel == p.Channel && ruled.ID == 0 {
			ruled = p
		} else if !ok && !p.IsNew && p.Channel != "102.1" && fresh.ID == 0 {
			fresh = p
		}
	}
	if ruled.ID == 0 || fresh.ID == 0 {
		t.Fatalf("fixture: ruled %d, fresh %d", ruled.ID, fresh.ID)
	}

	// Bob records Alice's rule airing: same row, still Alice's.
	id, _, existing, err := s.Record(ctx, ruled.ID, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := d.Recording(ctx, id)
	if !existing || r.OwnerID != alice.ID || r.RuleID == nil {
		t.Fatalf("after Bob's record: existing %v, owner %d, rule %v; want Alice's rule row untouched", existing, r.OwnerID, r.RuleID)
	}

	// A fresh airing is Bob's.
	id, _, existing, err = s.Record(ctx, fresh.ID, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := d.Recording(ctx, id); existing || r.OwnerID != bob.ID {
		t.Fatalf("fresh airing: existing %v, owner %d; want Bob's", existing, r.OwnerID)
	}

	// Bob cancels it and records it again: revived, still his.
	if err := d.CancelRecording(ctx, id); err != nil {
		t.Fatal(err)
	}
	again, _, existing, err := s.Record(ctx, fresh.ID, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := d.Recording(ctx, again); again != id || existing || r.Status != "scheduled" || r.OwnerID != bob.ID {
		t.Fatalf("re-record: id %d (was %d), existing %v, status %s, owner %d", again, id, existing, r.Status, r.OwnerID)
	}

	// Alice's rule airing, once cancelled, can be revived by Bob as his own,
	// and the rule no longer manages it.
	ruleRow := ruleRec[ruled.StartAt]
	if err := d.CancelRecording(ctx, ruleRow.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Record(ctx, ruled.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if r, _ := d.Recording(ctx, ruleRow.ID); r.OwnerID != bob.ID || r.RuleID != nil || r.Status != "scheduled" {
		t.Fatalf("revived rule airing: owner %d, rule %v, status %s", r.OwnerID, r.RuleID, r.Status)
	}
}
