package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/places"
	"github.com/tlmcguire/fistbump/backend/internal/store"
)

func (s *Server) radiusMatcher(ctx context.Context, origin string, miles int) (func(string) bool, error) {
	if miles < 1 || miles > places.MaxRadiusMiles {
		return nil, validationError("radius_miles", fmt.Sprintf("Enter a radius between 1 and %d miles.", places.MaxRadiusMiles))
	}
	if strings.TrimSpace(origin) == "" {
		profile, err := s.store.GetProfile(ctx)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if profile.HomeLocation != nil {
			origin = *profile.HomeLocation
		}
	}
	if strings.TrimSpace(origin) == "" {
		return nil, validationError("location", "Enter a city and state in Location or add one to your resume profile.")
	}
	match, found := places.Within(origin, miles)
	if !found {
		return nil, validationError("location", "Enter a recognized U.S. city and state, such as Charleston, SC, to search by distance.")
	}
	return match, nil
}

// locationRadius applies the same local radius filter to community listings.
func (s *Server) locationRadius(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Location    string   `json:"location"`
		RadiusMiles int      `json:"radius_miles"`
		Locations   []string `json:"locations"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return err
	}
	if len(body.Locations) > 500 {
		return validationError("locations", "A batch can contain at most 500 locations.")
	}
	match, err := s.radiusMatcher(r.Context(), body.Location, body.RadiusMiles)
	if err != nil {
		return err
	}
	matches := make([]bool, len(body.Locations))
	for i, location := range body.Locations {
		matches[i] = match(location)
	}
	writeJSON(w, http.StatusOK, struct {
		Matches []bool `json:"matches"`
	}{matches})
	return nil
}
