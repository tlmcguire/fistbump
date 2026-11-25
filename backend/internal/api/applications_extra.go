package api

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tlmcguire/fistbump/backend/internal/store"
)

// appExportCSV handles GET /v1/applications/export?format=csv: RFC 4180 CSV of all tracked applications.
func (s *Server) appExportCSV(w http.ResponseWriter, r *http.Request) error {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "csv"
	}
	if format != "csv" {
		return validationError("format", "format must be csv")
	}
	list, err := s.store.ListApplications(r.Context(), store.ApplicationFilter{Archived: "include"})
	if err != nil {
		return storeError(err, "Application")
	}
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{
		"ID", "Company", "Position", "Status", "Date Applied",
		"Next Step Date", "Archived", "Notes", "Created At", "Updated At",
	})
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	for _, a := range list {
		archived := "No"
		if a.ArchivedAt != nil {
			archived = "Yes"
		}
		_ = cw.Write([]string{
			strconv.FormatInt(a.ID, 10),
			a.CompanyName,
			a.PositionTitle,
			a.Status,
			deref(a.DateApplied),
			deref(a.NextStepDate),
			archived,
			deref(a.Notes),
			a.CreatedAt,
			a.UpdatedAt,
		})
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}
	h := w.Header()
	h.Set("Content-Type", "text/csv; charset=utf-8")
	h.Set("Content-Disposition", contentDisposition("Fistbump-Applications.csv"))
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
	return nil
}

// appExportICS handles GET /v1/applications/{id}/ics: RFC 5545 calendar invite for next_step_date.
func (s *Server) appExportICS(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	a, err := s.store.GetApplication(r.Context(), id)
	if err != nil {
		return storeError(err, "Application")
	}
	if a.NextStepDate == nil || strings.TrimSpace(*a.NextStepDate) == "" {
		return validationError("next_step_date", "Set a next step date before exporting a calendar event")
	}
	start, err := time.Parse("2006-01-02", strings.TrimSpace(*a.NextStepDate))
	if err != nil {
		return validationError("next_step_date", "Next step date must be YYYY-MM-DD")
	}
	end := start.AddDate(0, 0, 1)
	stamp := time.Now().UTC().Format("20060102T150405Z")
	summary := fmt.Sprintf("Follow-up: %s — %s", a.CompanyName, a.PositionTitle)
	desc := fmt.Sprintf("Status: %s", a.Status)
	if a.Notes != nil && strings.TrimSpace(*a.Notes) != "" {
		desc += "\\n" + escapeICS(strings.TrimSpace(*a.Notes))
	}
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Fistbump//Application Tracker//EN",
		"BEGIN:VEVENT",
		fmt.Sprintf("UID:fistbump-app-%d@local", a.ID),
		"DTSTAMP:" + stamp,
		"DTSTART;VALUE=DATE:" + start.Format("20060102"),
		"DTEND;VALUE=DATE:" + end.Format("20060102"),
		"SUMMARY:" + escapeICS(summary),
		"DESCRIPTION:" + desc,
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	name := fileName(a.CompanyName, a.PositionTitle, "Follow-up") + ".ics"
	h := w.Header()
	h.Set("Content-Type", "text/calendar; charset=utf-8")
	h.Set("Content-Disposition", contentDisposition(name))
	h.Set("Content-Length", strconv.Itoa(len(ics)))
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(ics))
	return nil
}

func escapeICS(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\r\n", `\n`, "\n", `\n`, ",", `\,`, ";", `\;`)
	return r.Replace(s)
}

// appStats handles GET /v1/applications/stats: stage funnel counts and recurring missing skills.
func (s *Server) appStats(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	archived := strings.TrimSpace(r.URL.Query().Get("archived"))
	if archived != "" && archived != "include" && archived != "only" {
		return validationError("archived", "archived must be empty, include, or only")
	}
	apps, err := s.store.ListApplications(ctx, store.ApplicationFilter{Archived: archived})
	if err != nil {
		return storeError(err, "Application")
	}
	counts := map[string]int{
		"Saved":        0,
		"Applied":      0,
		"Interviewing": 0,
		"Offered":      0,
		"Rejected":     0,
	}
	trackedJobs := make(map[int64]bool, len(apps))
	for _, a := range apps {
		counts[a.Status]++
		trackedJobs[a.JobID] = true
	}

	type skillGap struct {
		Skill string `json:"skill"`
		Count int    `json:"count"`
	}
	gaps := []skillGap{}
	if len(trackedJobs) > 0 {
		if master, err := s.store.GetMaster(ctx); err == nil && master.Profile != nil {
			freq := map[string]int{}
			for jobID := range trackedJobs {
				j, err := s.store.GetJob(ctx, jobID)
				if err != nil {
					continue
				}
				res := s.analyze(j, master)
				for _, m := range res.Required.Tech.Missing {
					freq[m]++
				}
			}
			for skill, count := range freq {
				gaps = append(gaps, skillGap{Skill: skill, Count: count})
			}
			sort.Slice(gaps, func(i, j int) bool {
				if gaps[i].Count != gaps[j].Count {
					return gaps[i].Count > gaps[j].Count
				}
				return gaps[i].Skill < gaps[j].Skill
			})
			if len(gaps) > 10 {
				gaps = gaps[:10]
			}
		}
	}

	writeJSON(w, http.StatusOK, struct {
		Total           int            `json:"total"`
		ByStage         map[string]int `json:"by_stage"`
		TopMissingSkill []skillGap     `json:"top_missing_skills"`
	}{
		Total:           len(apps),
		ByStage:         counts,
		TopMissingSkill: gaps,
	})
	return nil
}
