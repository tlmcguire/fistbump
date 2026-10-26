// Autocomplete for the search form: a dropdown under a text input, filled from recent searches
// first and then a fixed list. Items are shown with textContent only.
window.Suggest = (() => {
  const TITLES = [
    "Accountant", "Account Executive", "Account Manager", "Administrative Assistant", "Backend Engineer",
    "Business Analyst", "Business Development", "Content Writer", "Customer Success Manager",
    "Customer Support", "Data Analyst", "Data Engineer", "Data Scientist", "Database Administrator",
    "DevOps Engineer", "Engineering Manager", "Financial Analyst", "Frontend Engineer",
    "Full Stack Engineer", "Graphic Designer", "HR Generalist", "IT Support", "Machine Learning Engineer",
    "Marketing Manager", "Mobile Engineer", "Network Engineer", "Operations Analyst", "Product Designer",
    "Product Manager", "Program Manager", "Project Manager", "QA Engineer", "Recruiter",
    "Sales Development Representative", "Sales Engineer", "Security Analyst", "Security Engineer",
    "Site Reliability Engineer", "Software Engineer", "Solutions Engineer", "Systems Administrator",
    "Technical Program Manager", "Technical Writer", "UX Designer", "UX Researcher", "Web Developer",
    "Software Engineering Intern", "Data Analyst Intern", "Marketing Intern",
  ];

  // Roles that come in levels. Each gets junior, senior and numbered forms, so typing "junior dev" or
  // "analyst II" finds a suggestion.
  const LEVELED = [
    "Software Engineer", "Software Developer", "Web Developer", "Frontend Engineer", "Backend Engineer",
    "Full Stack Engineer", "Data Analyst", "Data Engineer", "Data Scientist", "Business Analyst",
    "Financial Analyst", "Security Analyst", "Security Engineer", "QA Engineer", "DevOps Engineer",
    "Systems Administrator", "Network Engineer", "Product Designer", "UX Designer", "Product Manager",
    "Project Manager", "Marketing Specialist", "Accountant", "Recruiter", "Customer Support Specialist",
  ];
  const LEVEL_PREFIXES = ["Junior", "Jr", "Entry Level", "Associate", "Senior", "Sr", "Staff", "Principal", "Lead"];
  const LEVEL_SUFFIXES = ["I", "II", "III", "Intern"];

  // Roles whose level is written as a tier or number.
  const TIERED = [
    "SOC Analyst Tier 1", "SOC Analyst Tier 2", "SOC Analyst Tier 3", "SOC Analyst I", "SOC Analyst II", "SOC Analyst III",
    "SOC 1", "SOC 2", "SOC 3", "Security Operations Analyst", "Incident Response Analyst", "Threat Hunter",
    "Help Desk Tier 1", "Help Desk Tier 2", "Service Desk Analyst", "Desktop Support Technician",
    "Technical Support Engineer Tier 1", "Technical Support Engineer Tier 2", "Support Engineer Level 1",
    "Support Engineer Level 2", "Customer Support Tier 1", "Customer Support Tier 2", "IT Technician",
    "New Grad Software Engineer", "Graduate Software Engineer", "Early Career Engineer", "Apprentice Developer",
    "Engineering Manager", "Senior Engineering Manager", "Director of Engineering", "VP of Engineering",
  ];

  const leveled = LEVELED.flatMap(role => [
    ...LEVEL_PREFIXES.map(prefix => `${prefix} ${role}`),
    ...LEVEL_SUFFIXES.map(suffix => `${role} ${suffix}`),
  ]);
  const ALL_TITLES = [...new Set([...TITLES, ...TIERED, ...leveled])];

  const PLACES = [
    "Albuquerque, NM", "Anchorage, AK", "Atlanta, GA", "Austin, TX", "Baltimore, MD", "Birmingham, AL",
    "Boise, ID", "Boston, MA", "Boulder, CO", "Brooklyn, NY", "Buffalo, NY", "Charleston, SC",
    "Charlotte, NC", "Chicago, IL", "Cincinnati, OH", "Cleveland, OH", "Columbia, SC", "Columbus, OH",
    "Dallas, TX", "Denver, CO", "Detroit, MI", "Durham, NC", "Fort Lauderdale, FL", "Greenville, SC",
    "Honolulu, HI", "Houston, TX", "Indianapolis, IN", "Jacksonville, FL", "Kansas City, MO",
    "Las Vegas, NV", "Los Angeles, CA", "Louisville, KY", "Memphis, TN", "Miami, FL", "Milwaukee, WI",
    "Minneapolis, MN", "Nashville, TN", "New Orleans, LA", "New York, NY", "Oakland, CA", "Oklahoma City, OK",
    "Omaha, NE", "Orlando, FL", "Philadelphia, PA", "Phoenix, AZ", "Pittsburgh, PA", "Portland, OR",
    "Providence, RI", "Raleigh, NC", "Richmond, VA", "Sacramento, CA", "Salt Lake City, UT",
    "San Antonio, TX", "San Diego, CA", "San Francisco, CA", "San Jose, CA", "Savannah, GA",
    "Seattle, WA", "St. Louis, MO", "Tampa, FL", "Tucson, AZ", "Washington, DC",
    "California", "Colorado", "Florida", "Georgia", "Illinois", "Massachusetts", "New York", "North Carolina",
    "Ohio", "Pennsylvania", "South Carolina", "Texas", "Virginia", "Washington", "United States",
  ];

  const MAX_ITEMS = 8;
  const MAX_RECENT = 8;

  const recent = key => {
    try {
      const list = JSON.parse(localStorage.getItem(`fistbump.recent.${key}`));
      return Array.isArray(list) ? list.filter(item => typeof item === "string") : [];
    } catch {
      return [];
    }
  };

  // remember puts a value first in the recent list for a field.
  const remember = (key, values) => {
    try {
      const list = recent(key);
      for (const value of [...values].reverse()) {
        if (!value) continue;
        const at = list.findIndex(item => item.toLowerCase() === value.toLowerCase());
        if (at >= 0) list.splice(at, 1);
        list.unshift(value);
      }
      localStorage.setItem(`fistbump.recent.${key}`, JSON.stringify(list.slice(0, MAX_RECENT)));
    } catch {
      // storage can be unavailable; suggestions still work from the fixed list
    }
  };

  // clearRecent forgets the recent entries of every box, so they stop appearing as suggestions.
  const clearRecent = () => {
    for (const key of ["keywords", "location", "company"]) {
      try {
        localStorage.removeItem(`fistbump.recent.${key}`);
      } catch {
        // storage can be unavailable; there is nothing to clear then
      }
    }
  };

  // matches ranks recent entries first, then names that start with the term, then names where every
  // typed word starts some word of the name ("junior dev" finds "Junior Developer"), then names that
  // contain the term.
  const matches = (term, key, fixed) => {
    const t = term.trim().toLowerCase();
    const typed = t.split(/\s+/).filter(Boolean);
    const seen = new Set();
    const out = [];
    const add = (value, show) => {
      const k = value.toLowerCase();
      if (!seen.has(k)) {
        seen.add(k);
        out.push({value, recent: show});
      }
    };
    const starts = value => value.toLowerCase().startsWith(t);
    const words = value => typed.every(word => value.toLowerCase().split(/[\s,/()-]+/).some(w => w.startsWith(word)));
    const contains = value => value.toLowerCase().includes(t);
    for (const value of recent(key)) if (starts(value)) add(value, true);
    for (const value of fixed) if (starts(value)) add(value, false);
    if (t) {
      for (const value of recent(key)) if (words(value)) add(value, true);
      for (const value of fixed) if (words(value)) add(value, false);
      for (const value of fixed) if (contains(value)) add(value, false);
    }
    return out.slice(0, MAX_ITEMS);
  };

  // attach adds the dropdown to an input. With list set, the input holds comma separated values and
  // only the part after the last comma is completed.
  const attach = (input, {key, fixed, list = false}) => {
    const box = document.createElement("ul");
    box.className = "suggestions";
    box.setAttribute("role", "listbox");
    box.hidden = true;
    input.insertAdjacentElement("afterend", box);
    input.setAttribute("autocomplete", "off");
    input.setAttribute("role", "combobox");
    input.setAttribute("aria-autocomplete", "list");
    input.setAttribute("aria-expanded", "false");

    let items = [];
    let active = -1;

    // The part of the input being completed, and the text before it.
    const split = () => {
      if (!list) return {head: "", term: input.value};
      const at = input.value.lastIndexOf(",");
      return {head: input.value.slice(0, at + 1) + (at >= 0 ? " " : ""), term: input.value.slice(at + 1)};
    };

    const close = () => {
      box.hidden = true;
      active = -1;
      input.setAttribute("aria-expanded", "false");
    };

    const highlight = index => {
      active = index;
      [...box.children].forEach((node, i) => {
        node.classList.toggle("active", i === index);
        node.setAttribute("aria-selected", i === index);
      });
    };

    const choose = index => {
      const {head} = split();
      input.value = head + items[index].value + (list ? ", " : "");
      close();
      input.focus();
    };

    const open = () => {
      items = matches(split().term, key, fixed);
      if (items.length === 0) return close();
      box.replaceChildren(...items.map((item, i) => {
        const node = document.createElement("li");
        node.setAttribute("role", "option");
        node.textContent = item.value;
        if (item.recent) {
          const tag = document.createElement("small");
          tag.textContent = "recent";
          node.append(tag);
        }
        // mousedown, not click, so the input keeps focus and blur does not close the list first
        node.addEventListener("mousedown", event => {
          event.preventDefault();
          choose(i);
        });
        return node;
      }));
      box.hidden = false;
      input.setAttribute("aria-expanded", "true");
      highlight(-1);
    };

    input.addEventListener("input", open);
    // The list opens when the user types or clicks into the box, not when the box is focused by the app.
    input.addEventListener("mousedown", () => { if (document.activeElement === input) open(); else setTimeout(open); });
    input.addEventListener("blur", close);
    input.addEventListener("keydown", event => {
      if (box.hidden) {
        if (event.key === "ArrowDown") open();
        return;
      }
      if (event.key === "ArrowDown") {
        event.preventDefault();
        highlight((active + 1) % items.length);
      } else if (event.key === "ArrowUp") {
        event.preventDefault();
        highlight((active - 1 + items.length) % items.length);
      } else if (event.key === "Enter" && active >= 0) {
        event.preventDefault(); // choose the highlighted item instead of submitting the form
        choose(active);
      } else if (event.key === "Enter") {
        close(); // the form submits; the list should not stay open over the results
      } else if (event.key === "Escape") {
        event.preventDefault(); // this Esc is used up: nothing else on the page should also close
        close();
      }
    });
  };

  return {attach, remember, clearRecent, matches, TITLES: ALL_TITLES, PLACES};
})();
