package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestNLPModeOnAPipePrintsHelp(t *testing.T) {
	testHome(t)
	var out, errb bytes.Buffer
	if code := Run([]string{"-m", "nlp"}, &out, &errb); code != 0 {
		t.Fatalf("code %d stderr %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "neuro-linguistic programming") {
		t.Fatalf("help %s", out.String())
	}
	if strings.Contains(out.String(), "bookmark") {
		t.Fatalf("help mentioned bookmarks: %s", out.String())
	}
}

func TestNLPModeRejectsRecordFlags(t *testing.T) {
	testHome(t)
	var out bytes.Buffer
	if code := Run([]string{"-m", "nlp", "-c"}, &out, &bytes.Buffer{}); code == 0 || !strings.Contains(out.String(), "Usage: crusade -m nlp") {
		t.Fatalf("code %d out %s", code, out.String())
	}
}
