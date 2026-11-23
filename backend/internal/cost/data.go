package cost

// The reference data behind the analysis. Every figure is a rounded estimate for a single adult,
// with the US national average at 100, meant to show how places compare rather than to quote a price.
// It ships with the app and is never fetched, so the analysis works offline and shares nothing.

// place is a city or a state: its cost of living, a typical one-bedroom rent and its income taxes.
type place struct {
	Name     string
	State    string  // two-letter code
	Index    float64 // cost of living, US average = 100
	Rent     int     // typical one-bedroom rent per month, USD
	StateTax float64 // effective state income tax on a middle-class salary, as a fraction
	CityTax  float64 // local income tax, as a fraction of pay
	Aliases  []string
}

// National figures, used when no place is known.
var national = place{Name: "the US average", Index: 100, Rent: 1500, StateTax: 0.04}

// metros are matched by name first, so "Austin, TX" gets Austin's rent rather than Texas's.
// The tax columns are filled in from the state table when a metro leaves them zero.
var metros = []place{
	{Name: "San Francisco, CA", State: "CA", Index: 175, Rent: 3000, Aliases: []string{"san francisco", "south san francisco", "san mateo", "daly city"}},
	{Name: "the San Francisco Bay Area, CA", State: "CA", Index: 165, Rent: 2800, Aliases: []string{"bay area", "san jose", "palo alto", "mountain view", "sunnyvale", "menlo park", "redwood city", "santa clara", "cupertino", "fremont", "los gatos", "foster city"}},
	{Name: "Oakland, CA", State: "CA", Index: 150, Rent: 2400, Aliases: []string{"oakland", "berkeley", "emeryville"}},
	{Name: "Los Angeles, CA", State: "CA", Index: 150, Rent: 2500, Aliases: []string{"los angeles", "santa monica", "culver city", "pasadena", "el segundo", "irvine", "orange county", "long beach"}},
	{Name: "San Diego, CA", State: "CA", Index: 145, Rent: 2500, Aliases: []string{"san diego", "carlsbad"}},
	{Name: "Sacramento, CA", State: "CA", Index: 118, Rent: 1800, Aliases: []string{"sacramento"}},
	{Name: "New York City, NY", State: "NY", Index: 187, Rent: 3600, CityTax: 0.035, StateTax: 0.055, Aliases: []string{"new york city", "new york", "nyc", "brooklyn", "manhattan", "queens", "bronx", "long island city"}},
	{Name: "Jersey City, NJ", State: "NJ", Index: 150, Rent: 2700, Aliases: []string{"jersey city", "hoboken", "newark"}},
	{Name: "Buffalo, NY", State: "NY", Index: 94, Rent: 1200, Aliases: []string{"buffalo", "rochester", "syracuse", "albany"}},
	{Name: "Boston, MA", State: "MA", Index: 150, Rent: 2900, Aliases: []string{"boston", "cambridge", "somerville", "waltham", "burlington, ma", "needham"}},
	{Name: "Seattle, WA", State: "WA", Index: 150, Rent: 2200, Aliases: []string{"seattle", "bellevue", "redmond", "kirkland", "tacoma"}},
	{Name: "Washington, DC", State: "DC", Index: 150, Rent: 2300, Aliases: []string{"washington, dc", "washington dc", "washington, d.c.", "arlington, va", "alexandria, va", "reston", "mclean", "tysons", "bethesda"}},
	{Name: "Chicago, IL", State: "IL", Index: 110, Rent: 1800, Aliases: []string{"chicago", "evanston", "naperville"}},
	{Name: "Austin, TX", State: "TX", Index: 105, Rent: 1500, Aliases: []string{"austin", "round rock"}},
	{Name: "Dallas, TX", State: "TX", Index: 102, Rent: 1450, Aliases: []string{"dallas", "plano", "frisco", "irving", "fort worth"}},
	{Name: "Houston, TX", State: "TX", Index: 96, Rent: 1300, Aliases: []string{"houston", "the woodlands"}},
	{Name: "San Antonio, TX", State: "TX", Index: 92, Rent: 1200, Aliases: []string{"san antonio"}},
	{Name: "Denver, CO", State: "CO", Index: 114, Rent: 1750, Aliases: []string{"denver", "aurora, co", "englewood, co"}},
	{Name: "Boulder, CO", State: "CO", Index: 125, Rent: 1900, Aliases: []string{"boulder"}},
	{Name: "Atlanta, GA", State: "GA", Index: 104, Rent: 1700, Aliases: []string{"atlanta", "alpharetta"}},
	{Name: "Miami, FL", State: "FL", Index: 120, Rent: 2300, Aliases: []string{"miami", "fort lauderdale", "boca raton"}},
	{Name: "Tampa, FL", State: "FL", Index: 100, Rent: 1750, Aliases: []string{"tampa", "st. petersburg", "clearwater"}},
	{Name: "Orlando, FL", State: "FL", Index: 98, Rent: 1650, Aliases: []string{"orlando"}},
	{Name: "Jacksonville, FL", State: "FL", Index: 94, Rent: 1450, Aliases: []string{"jacksonville"}},
	{Name: "Phoenix, AZ", State: "AZ", Index: 103, Rent: 1500, Aliases: []string{"phoenix", "scottsdale", "tempe", "chandler", "mesa"}},
	{Name: "Tucson, AZ", State: "AZ", Index: 94, Rent: 1100, Aliases: []string{"tucson"}},
	{Name: "Las Vegas, NV", State: "NV", Index: 102, Rent: 1400, Aliases: []string{"las vegas", "henderson"}},
	{Name: "Portland, OR", State: "OR", Index: 124, Rent: 1600, Aliases: []string{"portland, or", "portland, oregon", "beaverton", "hillsboro"}},
	{Name: "Salt Lake City, UT", State: "UT", Index: 106, Rent: 1400, Aliases: []string{"salt lake city", "lehi", "provo", "draper", "orem"}},
	{Name: "Minneapolis, MN", State: "MN", Index: 100, Rent: 1400, Aliases: []string{"minneapolis", "st. paul", "saint paul"}},
	{Name: "Philadelphia, PA", State: "PA", Index: 102, Rent: 1600, CityTax: 0.038, StateTax: 0.0307, Aliases: []string{"philadelphia"}},
	{Name: "Pittsburgh, PA", State: "PA", Index: 90, Rent: 1200, CityTax: 0.03, StateTax: 0.0307, Aliases: []string{"pittsburgh"}},
	{Name: "Baltimore, MD", State: "MD", Index: 94, Rent: 1500, Aliases: []string{"baltimore", "columbia, md"}},
	{Name: "Raleigh, NC", State: "NC", Index: 99, Rent: 1400, Aliases: []string{"raleigh", "durham", "cary", "chapel hill", "research triangle"}},
	{Name: "Charlotte, NC", State: "NC", Index: 98, Rent: 1500, Aliases: []string{"charlotte"}},
	{Name: "Nashville, TN", State: "TN", Index: 104, Rent: 1600, Aliases: []string{"nashville", "franklin, tn"}},
	{Name: "Memphis, TN", State: "TN", Index: 87, Rent: 1000, Aliases: []string{"memphis"}},
	{Name: "Columbus, OH", State: "OH", Index: 92, Rent: 1200, CityTax: 0.025, Aliases: []string{"columbus, oh", "columbus, ohio", "dublin, oh"}},
	{Name: "Cleveland, OH", State: "OH", Index: 88, Rent: 1000, CityTax: 0.025, Aliases: []string{"cleveland"}},
	{Name: "Cincinnati, OH", State: "OH", Index: 90, Rent: 1100, CityTax: 0.018, Aliases: []string{"cincinnati"}},
	{Name: "Detroit, MI", State: "MI", Index: 91, Rent: 1100, CityTax: 0.024, Aliases: []string{"detroit", "ann arbor", "troy, mi", "southfield"}},
	{Name: "Indianapolis, IN", State: "IN", Index: 91, Rent: 1200, Aliases: []string{"indianapolis"}},
	{Name: "Kansas City, MO", State: "MO", Index: 90, Rent: 1200, CityTax: 0.01, Aliases: []string{"kansas city"}},
	{Name: "St. Louis, MO", State: "MO", Index: 88, Rent: 1100, CityTax: 0.01, Aliases: []string{"st. louis", "saint louis", "st louis"}},
	{Name: "Milwaukee, WI", State: "WI", Index: 93, Rent: 1200, Aliases: []string{"milwaukee", "madison, wi"}},
	{Name: "Honolulu, HI", State: "HI", Index: 185, Rent: 2200, Aliases: []string{"honolulu"}},
	{Name: "Anchorage, AK", State: "AK", Index: 124, Rent: 1300, Aliases: []string{"anchorage"}},
	{Name: "Boise, ID", State: "ID", Index: 99, Rent: 1350, Aliases: []string{"boise"}},
	{Name: "Albuquerque, NM", State: "NM", Index: 94, Rent: 1100, Aliases: []string{"albuquerque"}},
	{Name: "Oklahoma City, OK", State: "OK", Index: 88, Rent: 1000, Aliases: []string{"oklahoma city", "tulsa"}},
	{Name: "Omaha, NE", State: "NE", Index: 91, Rent: 1100, Aliases: []string{"omaha"}},
	{Name: "New Orleans, LA", State: "LA", Index: 96, Rent: 1200, Aliases: []string{"new orleans"}},
	{Name: "Louisville, KY", State: "KY", Index: 90, Rent: 1100, CityTax: 0.022, Aliases: []string{"louisville"}},
	{Name: "Birmingham, AL", State: "AL", Index: 88, Rent: 1100, Aliases: []string{"birmingham, al"}},
	{Name: "Richmond, VA", State: "VA", Index: 96, Rent: 1400, Aliases: []string{"richmond, va", "richmond, virginia"}},
	{Name: "Providence, RI", State: "RI", Index: 112, Rent: 1700, Aliases: []string{"providence"}},
	{Name: "Charleston, SC", State: "SC", Index: 106, Rent: 1600, Aliases: []string{"charleston, sc"}},
	{Name: "Greenville, SC", State: "SC", Index: 94, Rent: 1200, Aliases: []string{"greenville, sc"}},
	{Name: "Savannah, GA", State: "GA", Index: 98, Rent: 1300, Aliases: []string{"savannah"}},
}

// states are the fallback when a posting names a state but not a city.
var states = []place{
	{Name: "Alabama", State: "AL", Index: 89, Rent: 950, StateTax: 0.04},
	{Name: "Alaska", State: "AK", Index: 124, Rent: 1200},
	{Name: "Arizona", State: "AZ", Index: 107, Rent: 1250, StateTax: 0.025},
	{Name: "Arkansas", State: "AR", Index: 88, Rent: 900, StateTax: 0.039},
	{Name: "California", State: "CA", Index: 142, Rent: 2000, StateTax: 0.06},
	{Name: "Colorado", State: "CO", Index: 105, Rent: 1500, StateTax: 0.044},
	{Name: "Connecticut", State: "CT", Index: 112, Rent: 1600, StateTax: 0.05},
	{Name: "Delaware", State: "DE", Index: 102, Rent: 1300, StateTax: 0.05},
	{Name: "District of Columbia", State: "DC", Index: 150, Rent: 2300, StateTax: 0.075},
	{Name: "Florida", State: "FL", Index: 102, Rent: 1500},
	{Name: "Georgia", State: "GA", Index: 94, Rent: 1250, StateTax: 0.052},
	{Name: "Hawaii", State: "HI", Index: 184, Rent: 2000, StateTax: 0.065},
	{Name: "Idaho", State: "ID", Index: 99, Rent: 1100, StateTax: 0.053},
	{Name: "Illinois", State: "IL", Index: 94, Rent: 1250, StateTax: 0.045},
	{Name: "Indiana", State: "IN", Index: 90, Rent: 1000, StateTax: 0.03},
	{Name: "Iowa", State: "IA", Index: 90, Rent: 900, StateTax: 0.04},
	{Name: "Kansas", State: "KS", Index: 91, Rent: 950, StateTax: 0.045},
	{Name: "Kentucky", State: "KY", Index: 92, Rent: 950, StateTax: 0.04},
	{Name: "Louisiana", State: "LA", Index: 93, Rent: 1000, StateTax: 0.035},
	{Name: "Maine", State: "ME", Index: 111, Rent: 1300, StateTax: 0.06},
	{Name: "Maryland", State: "MD", Index: 115, Rent: 1600, StateTax: 0.05},
	{Name: "Massachusetts", State: "MA", Index: 148, Rent: 2300, StateTax: 0.05},
	{Name: "Michigan", State: "MI", Index: 92, Rent: 1000, StateTax: 0.0425},
	{Name: "Minnesota", State: "MN", Index: 98, Rent: 1200, StateTax: 0.06},
	{Name: "Mississippi", State: "MS", Index: 85, Rent: 850, StateTax: 0.04},
	{Name: "Missouri", State: "MO", Index: 89, Rent: 1000, StateTax: 0.045},
	{Name: "Montana", State: "MT", Index: 103, Rent: 1050, StateTax: 0.05},
	{Name: "Nebraska", State: "NE", Index: 91, Rent: 1000, StateTax: 0.05},
	{Name: "Nevada", State: "NV", Index: 104, Rent: 1300},
	{Name: "New Hampshire", State: "NH", Index: 115, Rent: 1500},
	{Name: "New Jersey", State: "NJ", Index: 114, Rent: 1800, StateTax: 0.05},
	{Name: "New Mexico", State: "NM", Index: 92, Rent: 1000, StateTax: 0.04},
	{Name: "New York", State: "NY", Index: 125, Rent: 1800, StateTax: 0.055},
	{Name: "North Carolina", State: "NC", Index: 96, Rent: 1200, StateTax: 0.0425},
	{Name: "North Dakota", State: "ND", Index: 94, Rent: 900, StateTax: 0.02},
	{Name: "Ohio", State: "OH", Index: 91, Rent: 1000, StateTax: 0.03},
	{Name: "Oklahoma", State: "OK", Index: 87, Rent: 900, StateTax: 0.04},
	{Name: "Oregon", State: "OR", Index: 115, Rent: 1400, StateTax: 0.075},
	{Name: "Pennsylvania", State: "PA", Index: 97, Rent: 1200, StateTax: 0.0307},
	{Name: "Rhode Island", State: "RI", Index: 111, Rent: 1500, StateTax: 0.045},
	{Name: "South Carolina", State: "SC", Index: 95, Rent: 1100, StateTax: 0.05},
	{Name: "South Dakota", State: "SD", Index: 94, Rent: 850},
	{Name: "Tennessee", State: "TN", Index: 91, Rent: 1250},
	{Name: "Texas", State: "TX", Index: 92, Rent: 1250},
	{Name: "Utah", State: "UT", Index: 104, Rent: 1250, StateTax: 0.045},
	{Name: "Vermont", State: "VT", Index: 114, Rent: 1300, StateTax: 0.055},
	{Name: "Virginia", State: "VA", Index: 101, Rent: 1400, StateTax: 0.05},
	{Name: "Washington", State: "WA", Index: 112, Rent: 1600},
	{Name: "West Virginia", State: "WV", Index: 87, Rent: 800, StateTax: 0.045},
	{Name: "Wisconsin", State: "WI", Index: 94, Rent: 1050, StateTax: 0.05},
	{Name: "Wyoming", State: "WY", Index: 94, Rent: 900},
}

// role is a family of jobs with a typical yearly base salary for a mid-level hire at the national
// average cost of living. Patterns are checked in order, so the specific ones come first.
type role struct {
	Name    string
	Words   []string // any of these in the title (lower case) puts the posting in the family
	Median  int
	Ladders bool // the title's level (junior, senior, staff) moves the pay; managers are paid by scope instead
}

var roles = []role{
	{"Machine learning engineer", []string{"machine learning", "ml engineer", "ai engineer", "applied scientist", "research engineer"}, 165000, true},
	{"Engineering manager", []string{"engineering manager", "director of engineering", "head of engineering", "vp of engineering", "vp, engineering"}, 190000, false},
	{"Technical program manager", []string{"technical program manager", "tpm"}, 150000, true},
	{"Data scientist", []string{"data scientist"}, 138000, true},
	{"Data engineer", []string{"data engineer", "analytics engineer"}, 135000, true},
	{"Data analyst", []string{"data analyst", "analytics analyst", "bi analyst", "business intelligence"}, 85000, true},
	{"Business analyst", []string{"business analyst", "systems analyst", "operations analyst"}, 85000, true},
	{"DevOps or site reliability engineer", []string{"devops", "site reliability", "sre", "platform engineer", "cloud engineer", "infrastructure engineer"}, 140000, true},
	{"Security analyst", []string{"soc analyst", "security analyst", "security operations", "incident response", "threat", "vulnerability analyst", "compliance analyst"}, 100000, true},
	{"Security engineer", []string{"security engineer", "application security", "appsec", "cybersecurity engineer", "penetration"}, 140000, true},
	{"QA engineer", []string{"qa engineer", "quality assurance", "test engineer", "sdet", "software engineer in test"}, 100000, true},
	{"Software engineer", []string{"software engineer", "software developer", "developer", "frontend", "front-end", "front end", "backend", "back-end", "back end", "full stack", "full-stack", "fullstack", "mobile engineer", "ios engineer", "android engineer", "web developer", "swe", "programmer"}, 135000, true},
	{"Product manager", []string{"product manager", "product owner", "group product", "head of product"}, 140000, true},
	{"Program or project manager", []string{"program manager", "project manager", "scrum master", "delivery manager"}, 105000, true},
	{"Product designer", []string{"product designer", "ux designer", "ui designer", "ux/ui", "ui/ux", "interaction designer", "visual designer", "design lead"}, 118000, true},
	{"UX researcher", []string{"ux researcher", "user researcher", "design researcher"}, 120000, true},
	{"Solutions or sales engineer", []string{"solutions engineer", "sales engineer", "solutions architect", "customer engineer", "field engineer"}, 135000, true},
	{"Sales development representative", []string{"sales development", "business development representative", "sdr", "bdr"}, 65000, true},
	{"Account executive", []string{"account executive", "account manager", "sales manager", "business development"}, 110000, true},
	{"Customer success manager", []string{"customer success", "client success", "implementation"}, 85000, true},
	{"Support engineer", []string{"support engineer", "technical support", "application support"}, 88000, true},
	{"Customer support", []string{"customer support", "customer service", "customer care", "support specialist", "support representative"}, 58000, true},
	{"Marketing manager", []string{"marketing manager", "product marketing", "growth manager", "demand generation", "brand manager", "marketing specialist", "marketing"}, 105000, true},
	{"Technical writer", []string{"technical writer", "documentation", "content designer"}, 90000, true},
	{"Content writer", []string{"content writer", "copywriter", "content strategist", "editor"}, 75000, true},
	{"Recruiter or HR", []string{"recruiter", "talent acquisition", "hr generalist", "human resources", "people operations", "people partner"}, 85000, true},
	{"Financial analyst", []string{"financial analyst", "fp&a", "finance analyst"}, 92000, true},
	{"Accountant", []string{"accountant", "accounting", "controller", "bookkeeper"}, 80000, true},
	{"IT support", []string{"help desk", "helpdesk", "service desk", "desktop support", "it support", "it technician", "it specialist"}, 58000, true},
	{"Systems or network administrator", []string{"systems administrator", "sysadmin", "network engineer", "network administrator", "it administrator"}, 95000, true},
	{"Database administrator", []string{"database administrator", "dba"}, 112000, true},
	{"Graphic designer", []string{"graphic designer", "brand designer", "motion designer"}, 70000, true},
	{"Operations", []string{"operations manager", "operations coordinator", "business operations", "program coordinator", "administrative", "office manager"}, 72000, true},
}

// levelFactor scales a mid-level salary by the level the title names.
var levelFactor = map[string]float64{"Intern": 0.45, "Entry": 0.74, "Mid": 1.0, "Senior": 1.27, "Lead": 1.5}
