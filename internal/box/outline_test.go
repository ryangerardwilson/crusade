package box

import "testing"

func TestParseOutlineNestsAndDerivesIDs(t *testing.T) {
	outline, err := ParseOutline("Move\nWork\n  Email\n  Deep work\nPlan tomorrow\n")
	if err != nil {
		t.Fatal(err)
	}
	email, ok := outline.ItemByID("work/email")
	if !ok || email.Label != "Email" {
		t.Fatalf("email = %#v ok=%v", email, ok)
	}
	if outline.LeafCount() != 4 {
		t.Fatalf("leaves = %d", outline.LeafCount())
	}
	checked := map[string]bool{"work/email": true}
	if outline.StatusFor("work", checked) != Partial {
		t.Fatal("parent should be partial")
	}
	next, err := Toggle(outline, checked, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !next["work/email"] || !next["work/deep-work"] || outline.StatusFor("work", next) != Checked {
		t.Fatalf("checked = %#v", next)
	}
	cleared, err := ApplyCheck(outline, next, "work", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 0 {
		t.Fatalf("cleared = %#v", cleared)
	}
}

func TestParseOutlineRejectsDuplicateIDs(t *testing.T) {
	if _, err := ParseOutline("Work\nWork\n"); err == nil {
		t.Fatal("expected duplicate id")
	}
}
