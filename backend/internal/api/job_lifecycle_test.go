package api

import (
	"fmt"
	"testing"
)

func TestJobArchiveWithApplication(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	job := trackableJob(t, s)
	app := trackJob(t, s, job)
	out := expect(t, call(t, s, "POST", fmt.Sprintf("/v1/jobs/%d/archive", job), `{"archive_application":true}`), 200)
	if out["archived_at"] == nil {
		t.Errorf("job = %v, want archived_at set", out)
	}
	if a := expect(t, call(t, s, "GET", fmt.Sprintf("/v1/applications/%d", app), ""), 200); a["archived_at"] == nil {
		t.Errorf("application = %v, want archived too", a)
	}
}

func TestJobArchiveWithoutBody(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	job := trackableJob(t, s)
	out := expect(t, call(t, s, "POST", fmt.Sprintf("/v1/jobs/%d/archive", job), ""), 200)
	if out["archived_at"] == nil {
		t.Errorf("job = %v, want archived_at set", out)
	}
}

func TestJobUnarchiveRestoresApplication(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	job := trackableJob(t, s)
	app := trackJob(t, s, job)
	expect(t, call(t, s, "POST", fmt.Sprintf("/v1/jobs/%d/archive", job), `{"archive_application":true}`), 200)
	out := expect(t, call(t, s, "POST", fmt.Sprintf("/v1/jobs/%d/unarchive", job), ""), 200)
	if out["archived_at"] != nil {
		t.Errorf("job = %v, want archived_at null", out)
	}
	if a := expect(t, call(t, s, "GET", fmt.Sprintf("/v1/applications/%d", app), ""), 200); a["archived_at"] != nil {
		t.Errorf("application = %v, want archived_at null", a)
	}
}

func TestJobTrash(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	free := trackableJob(t, s)
	out := expect(t, call(t, s, "POST", fmt.Sprintf("/v1/jobs/%d/trash", free), ""), 200)
	if out["trashed_at"] == nil {
		t.Errorf("job = %v, want trashed_at set", out)
	}

	tracked := trackableJob(t, s)
	app := trackJob(t, s, tracked)
	out = expect(t, call(t, s, "POST", fmt.Sprintf("/v1/jobs/%d/trash", tracked), ""), 409)
	details := out["error"].(map[string]any)["details"].(map[string]any)
	if details["application_id"] != float64(app) {
		t.Errorf("details = %v, want application_id %d", details, app)
	}
}

func TestJobRestore(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	job := trackableJob(t, s)
	expect(t, call(t, s, "POST", fmt.Sprintf("/v1/jobs/%d/trash", job), ""), 200)
	out := expect(t, call(t, s, "POST", fmt.Sprintf("/v1/jobs/%d/restore", job), ""), 200)
	if out["trashed_at"] != nil {
		t.Errorf("job = %v, want trashed_at null", out)
	}
	if n := len(expect2(t, call(t, s, "GET", "/v1/jobs?trashed=only", ""))); n != 0 {
		t.Errorf("trash has %d after restore, want 0", n)
	}
}

func TestJobLifecycleUnknownJob(t *testing.T) {
	s := newJobServer(t, &fakeFetcher{})
	for _, action := range []string{"archive", "unarchive", "trash", "restore"} {
		expect(t, call(t, s, "POST", fmt.Sprintf("/v1/jobs/999999/%s", action), ""), 404)
	}
}
