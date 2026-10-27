// Checks GitHub Releases for a newer version. Nothing is downloaded or installed: the user is shown the
// release page. Builds are unsigned for now, so installing an update in place is left for later.
const RELEASES_API = 'https://api.github.com/repos/tlmcguire/fistbump/releases/latest';
const RELEASES_PAGE = 'https://github.com/tlmcguire/fistbump/releases';

// parseVersion reads "v1.2.3" or "1.2.3-beta.1" into numbers and a pre-release tag, or null.
function parseVersion(text) {
  const m = /^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?$/.exec(String(text || '').trim());
  return m ? { parts: [Number(m[1]), Number(m[2]), Number(m[3])], pre: m[4] || '' } : null;
}

// compareVersions is negative when a is older than b. A pre-release is older than its release.
function compareVersions(a, b) {
  const x = parseVersion(a);
  const y = parseVersion(b);
  if (!x || !y) return 0;
  for (let i = 0; i < 3; i++) {
    if (x.parts[i] !== y.parts[i]) return x.parts[i] - y.parts[i];
  }
  if (x.pre === y.pre) return 0;
  if (!x.pre) return 1;
  if (!y.pre) return -1;
  return x.pre < y.pre ? -1 : 1;
}

// checkForUpdates returns { status, current, latest, url, publishedAt, message }. status is "available",
// "current", "none" (no release published yet) or "error". fetchImpl is replaceable for tests.
async function checkForUpdates(current, fetchImpl = fetch) {
  const base = { current, latest: '', url: RELEASES_PAGE, publishedAt: '' };
  let res;
  try {
    res = await fetchImpl(RELEASES_API, {
      headers: { Accept: 'application/vnd.github+json', 'User-Agent': `fistbump/${current}` },
      signal: AbortSignal.timeout(10_000),
    });
  } catch {
    return { ...base, status: 'error', message: 'Could not reach GitHub. Check your connection and try again.' };
  }
  if (res.status === 404) return { ...base, status: 'none', message: 'No release has been published yet.' };
  if (!res.ok) return { ...base, status: 'error', message: `GitHub answered with an error (${res.status}). Try again later.` };

  let release;
  try {
    release = await res.json();
  } catch {
    return { ...base, status: 'error', message: 'GitHub sent an unreadable answer. Try again later.' };
  }
  const latest = String(release.tag_name || '').replace(/^v/, '');
  const url = typeof release.html_url === 'string' && release.html_url.startsWith('https://github.com/') ? release.html_url : RELEASES_PAGE;
  const found = { ...base, latest, url, publishedAt: release.published_at || '' };
  if (!parseVersion(latest)) return { ...found, status: 'error', message: 'The latest release has a version this app cannot read.' };
  return compareVersions(current, latest) < 0
    ? { ...found, status: 'available', message: `Fistbump ${latest} is available.` }
    : { ...found, status: 'current', message: `You have the latest version (${current}).` };
}

module.exports = { checkForUpdates, compareVersions, parseVersion, RELEASES_PAGE };
