package pdf

import (
	"strings"
	"testing"
)

func TestAuditDetectsOnePageAndWarnings(t *testing.T) {
	short := "# Jane Doe\n\njane@example.com\n\n## Experience\n\n### Engineer, Acme\n\n- Reduced build times by 45% across 12 services.\n"
	res := Audit(short, StyleClassic)
	if res.PageCount != 1 {
		t.Fatalf("expected 1 page, got %d", res.PageCount)
	}
	if res.BulletCount != 1 || res.QuantifiedCount != 1 {
		t.Fatalf("expected 1 quantified bullet, got %+v", res)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", res.Warnings)
	}

	noEmail := "# Jane Doe\n\n## Experience\n\n### Engineer, Acme\n\n- Built APIs.\n- Fixed bugs.\n- Reviewed code.\n"
	res2 := Audit(noEmail, StyleClassic)
	joined := strings.Join(res2.Warnings, " | ")
	if !strings.Contains(joined, "email") || !strings.Contains(joined, "metrics") {
		t.Fatalf("expected email and metrics warnings, got %v", res2.Warnings)
	}
}
