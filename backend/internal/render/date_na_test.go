package render

import (
	"strings"
	"testing"

	"github.com/tlmcguire/fistbump/backend/internal/models"
)

func TestMasterOmitsDatesWhenDateNAIsTrue(t *testing.T) {
	start := "2022-01-01"
	loc := "Remote"
	desc := "- Maintained open-source CLI."
	md := Master(models.Master{
		Profile: &models.Profile{FullName: "Ada Lovelace"},
		Experiences: []models.Experience{
			{
				CompanyName: "Open Source",
				JobTitle:    "Maintainer",
				StartDate:   &start,
				EndDate:     nil,
				DateNA:      true,
				Location:    &loc,
				Description: &desc,
			},
		},
	})
	if strings.Contains(md, "2022") || strings.Contains(md, "Present") {
		t.Errorf("expected dates to be omitted when DateNA is true, got:\n%s", md)
	}
	if !strings.Contains(md, "Remote") {
		t.Errorf("expected Location to still be rendered when DateNA is true, got:\n%s", md)
	}
}
