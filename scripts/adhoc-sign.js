// An afterPack hook for electron-builder. It ad-hoc signs the FistBump app that the build just
// packed, and nothing else. electron-builder's own "-" identity matches any keychain certificate
// whose name contains a dash, so it can sign with an unrelated certificate. This hook never looks
// at the keychain.
const { execFileSync } = require('node:child_process');
const path = require('node:path');

module.exports = async function adhocSign(context) {
  if (context.electronPlatformName !== 'darwin') return;
  const app = path.join(context.appOutDir, `${context.packager.appInfo.productFilename}.app`);
  execFileSync('codesign', ['--force', '--deep', '--sign', '-', app], { stdio: 'inherit' });
  console.log(`ad-hoc signed ${app}`);
};
