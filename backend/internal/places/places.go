// Package places matches job locations to Census place coordinates without sending queries online.
package places

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// 2025 U.S. Census Gazetteer national places file, reduced to state, name, and coordinates.
// Source: https://www.census.gov/geographies/reference-files/time-series/geo/gazetteer-files.2025.html
//
//go:embed us-places.tsv.gz
var gazetteer []byte

const MaxRadiusMiles = 5000

type point struct{ lat, lon float64 }

type lookup struct {
	qualified map[string]point
	unique    map[string]point
}

var cities = sync.OnceValue(readGazetteer)

func normalize(text string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

func readGazetteer() lookup {
	data := lookup{qualified: make(map[string]point), unique: make(map[string]point)}
	duplicates := make(map[string]bool)
	r, err := gzip.NewReader(bytes.NewReader(gazetteer))
	if err != nil {
		panic(err)
	}
	defer r.Close()
	rows := bufio.NewScanner(r)
	for rows.Scan() {
		fields := strings.Split(rows.Text(), "\t")
		lat, errLat := strconv.ParseFloat(fields[2], 64)
		lon, errLon := strconv.ParseFloat(fields[3], 64)
		if errLat != nil || errLon != nil {
			panic("invalid Census gazetteer coordinates")
		}
		name, state, p := normalize(fields[1]), fields[0], point{lat, lon}
		data.qualified[name+" "+state] = p
		if _, seen := data.unique[name]; seen {
			duplicates[name] = true
		}
		data.unique[name] = p
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	for name := range duplicates {
		delete(data.unique, name)
	}
	return data
}

var placeAliases = map[string]string{
	"nyc": "new york ny", "new york city": "new york ny",
	"san francisco bay area": "san francisco ca", "sf": "san francisco ca",
	"district of columbia": "washington dc", "philly": "philadelphia pa",
}

var countrySuffixes = []string{" united states of america", " united states", " usa", " us"}
var zipSuffix = regexp.MustCompile(` [0-9]{5}(?: [0-9]{4})?$`)
var modeSuffix = regexp.MustCompile(`(?i)\s*[(-]?\s*\b(?:remote|hybrid|on[ -]?site)\b\s*\)?$`)
var postingSeparator = regexp.MustCompile(`\s*[;|\n•·]\s*|\s+/\s+`)
var consecutivePlaces = regexp.MustCompile(`(?i)^(.*?,\s*[A-Z]{2})(?:,\s*)(.+)$`)

func resolve(raw string) (point, bool) {
	raw = strings.TrimSpace(modeSuffix.ReplaceAllString(strings.TrimSpace(raw), ""))
	// Keep ZIP codes out of the city lookup. The radius uses representative place points.
	key := normalize(raw)
	for _, suffix := range countrySuffixes {
		key = strings.TrimSuffix(key, suffix)
	}
	key = zipSuffix.ReplaceAllString(key, "")
	if alias, ok := placeAliases[key]; ok {
		key = alias
	}
	data := cities()
	if p, ok := data.qualified[key]; ok {
		return p, true
	}
	for name, code := range stateCodes {
		if city, ok := strings.CutSuffix(key, " "+name); ok {
			if p, found := data.qualified[city+" "+code]; found {
				return p, true
			}
		}
	}
	p, ok := data.unique[key]
	return p, ok
}

// Within returns a matcher for listings with at least one resolvable place within miles.
// An ambiguous city name must be qualified with its state.
func Within(origin string, miles int) (func(string) bool, bool) {
	center, ok := resolve(origin)
	if !ok {
		return nil, false
	}
	return func(location string) bool {
		for _, segment := range postingSeparator.Split(location, -1) {
			if p, found := resolve(segment); found {
				if distanceMiles(center, p) <= float64(miles) {
					return true
				}
				continue
			}
			// ATS feeds occasionally use a comma between complete City, ST pairs.
			for {
				match := consecutivePlaces.FindStringSubmatch(segment)
				if match == nil {
					break
				}
				if p, found := resolve(match[1]); found && distanceMiles(center, p) <= float64(miles) {
					return true
				}
				segment = match[2]
			}
			if p, found := resolve(segment); found && distanceMiles(center, p) <= float64(miles) {
				return true
			}
		}
		return false
	}, true
}

func distanceMiles(a, b point) float64 {
	const rad = math.Pi / 180
	lat := math.Sin((b.lat - a.lat) * rad / 2)
	lon := math.Sin((b.lon - a.lon) * rad / 2)
	h := lat*lat + math.Cos(a.lat*rad)*math.Cos(b.lat*rad)*lon*lon
	return 3958.76 * 2 * math.Asin(math.Sqrt(min(h, 1)))
}

var stateCodes = map[string]string{
	"alabama": "al", "alaska": "ak", "arizona": "az", "arkansas": "ar", "california": "ca", "colorado": "co",
	"connecticut": "ct", "delaware": "de", "district of columbia": "dc", "florida": "fl", "georgia": "ga",
	"hawaii": "hi", "idaho": "id", "illinois": "il", "indiana": "in", "iowa": "ia", "kansas": "ks",
	"kentucky": "ky", "louisiana": "la", "maine": "me", "maryland": "md", "massachusetts": "ma",
	"michigan": "mi", "minnesota": "mn", "mississippi": "ms", "missouri": "mo", "montana": "mt",
	"nebraska": "ne", "nevada": "nv", "new hampshire": "nh", "new jersey": "nj", "new mexico": "nm",
	"new york": "ny", "north carolina": "nc", "north dakota": "nd", "ohio": "oh", "oklahoma": "ok",
	"oregon": "or", "pennsylvania": "pa", "rhode island": "ri", "south carolina": "sc", "south dakota": "sd",
	"tennessee": "tn", "texas": "tx", "utah": "ut", "vermont": "vt", "virginia": "va", "washington": "wa",
	"west virginia": "wv", "wisconsin": "wi", "wyoming": "wy", "puerto rico": "pr",
}
