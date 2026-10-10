// Node can run under Rosetta on Apple Silicon while Electron runs natively. Scripts use the real CPU,
// which is what Electron's process.arch reports.
const { spawnSync } = require('child_process');

module.exports = function hostArch() {
  if (process.platform === 'darwin' && process.arch === 'x64') {
    const r = spawnSync('sysctl', ['-n', 'sysctl.proc_translated'], { encoding: 'utf8' });
    if (r.status === 0 && r.stdout.trim() === '1') return 'arm64';
  }
  return process.arch;
};
