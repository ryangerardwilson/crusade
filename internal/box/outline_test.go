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

func TestInsertRenameAndDeleteKeepChildrenAndIDs(t *testing.T) {
	outline, err := ParseOutline("Move\nWork\n  Email\n")
	if err != nil {
		t.Fatal(err)
	}
	outline, id, err := outline.InsertAfter("work/email", "Deep work")
	if err != nil || id != "work/deep-work" {
		t.Fatalf("id=%s err=%v", id, err)
	}
	flat := outline.Flatten()
	if len(flat) != 4 || flat[3].ID != "work/deep-work" || flat[3].Depth != 1 {
		t.Fatalf("flat = %#v", flat)
	}
	renamed, err := outline.Rename("work", "Job")
	if err != nil {
		t.Fatal(err)
	}
	job, ok := renamed.ItemByID("work")
	if !ok || job.Label != "Job" || len(job.Children) != 2 {
		t.Fatalf("renamed = %#v ok=%v", job, ok)
	}
	deleted, err := renamed.Delete("work")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := deleted.ItemByID("work/email"); ok || len(deleted.Flatten()) != 1 {
		t.Fatalf("deleted = %#v", deleted.Flatten())
	}
	if _, _, err := outline.InsertAfter("move", "Move"); err == nil {
		t.Fatal("expected duplicate box")
	}
}
