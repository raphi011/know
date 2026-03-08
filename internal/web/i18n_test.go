package web

import "testing"

func TestT_English(t *testing.T) {
	tr := T("en")

	got := tr("app.title")
	if got != "Knowhow" {
		t.Errorf("T(en)(app.title) = %q, want %q", got, "Knowhow")
	}
}

func TestT_German(t *testing.T) {
	tr := T("de")

	got := tr("nav.agent")
	if got != "Agent" {
		t.Errorf("T(de)(nav.agent) = %q, want %q", got, "Agent")
	}
}

func TestT_FallbackToEnglish(t *testing.T) {
	tr := T("fr") // unsupported locale

	got := tr("app.title")
	if got != "Knowhow" {
		t.Errorf("T(fr)(app.title) = %q, want %q (should fallback to en)", got, "Knowhow")
	}
}

func TestT_MissingKey(t *testing.T) {
	tr := T("en")

	got := tr("nonexistent.key")
	if got != "[nonexistent.key]" {
		t.Errorf("T(en)(nonexistent.key) = %q, want %q", got, "[nonexistent.key]")
	}
}
