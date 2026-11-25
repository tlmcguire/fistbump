// Regression tests for Tauri v2 packaging, cross-WebView visuals, IPC channel coverage, and resume UI.
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');

const root = path.join(__dirname, '..', '..');
const tauriDir = path.join(root, 'src-tauri');
const rendererDir = path.join(root, 'app', 'renderer');
const conf = JSON.parse(fs.readFileSync(path.join(tauriDir, 'tauri.conf.json'), 'utf8'));

test('tauri.conf.json bundle identifier avoids .app suffix and updater is configured', () => {
  assert.ok(conf.identifier && !conf.identifier.endsWith('.app'), 'macOS bundle identifier must not end with .app');
  assert.strictEqual(conf.bundle?.createUpdaterArtifacts, true);
  assert.ok(conf.plugins?.updater?.pubkey?.trim().length > 20, 'updater pubkey should be populated');
});

test('macOS DMG config matches 540x380 background artwork and symmetric icon positions', () => {
  const dmg = conf.bundle?.macOS?.dmg;
  assert.ok(dmg, 'bundle.macOS.dmg must be configured');
  assert.deepEqual(dmg.windowSize, { width: 540, height: 380 });
  assert.strictEqual(dmg.appPosition.x + dmg.applicationFolderPosition.x, 540);
  assert.strictEqual(dmg.appPosition.y, dmg.applicationFolderPosition.y);
  assert.ok(fs.existsSync(path.resolve(tauriDir, dmg.background)), `missing DMG background: ${dmg.background}`);
});

test('Windows NSIS and Linux/desktop icon assets all exist on disk', () => {
  const nsis = conf.bundle?.windows?.nsis;
  assert.ok(nsis?.sidebarImage, 'NSIS sidebarImage must be set');
  assert.ok(nsis?.installerIcon, 'NSIS installerIcon must be set');
  assert.ok(fs.existsSync(path.resolve(tauriDir, nsis.sidebarImage)), 'missing NSIS sidebarImage');
  assert.ok(fs.existsSync(path.resolve(tauriDir, nsis.installerIcon)), 'missing NSIS installerIcon');

  const icons = conf.bundle?.icon || [];
  for (const required of ['icons/32x32.png', 'icons/128x128.png', 'icons/128x128@2x.png', 'icons/512x512.png']) {
    assert.ok(icons.includes(required), `bundle.icon missing Linux hicolor icon: ${required}`);
  }
  for (const iconPath of icons) {
    assert.ok(fs.existsSync(path.resolve(tauriDir, iconPath)), `missing bundle icon: ${iconPath}`);
  }
  assert.ok(fs.existsSync(path.join(tauriDir, 'resources', 'bin', '.gitkeep')), 'src-tauri/resources/bin/.gitkeep must exist');
});

test('Tailor thinking mascot uses transparent APNG instead of VP9 WebM video', () => {
  const diffView = fs.readFileSync(path.join(rendererDir, 'views', 'diff-view', 'diff-view.js'), 'utf8');
  assert.ok(!diffView.includes('.webm'), 'diff-view.js must not reference .webm (broken alpha in WKWebView)');
  assert.ok(diffView.includes('thinking-anim.png'), 'diff-view.js should render thinking-anim.png');

  const apngPath = path.join(rendererDir, 'views', 'graphics', 'mascots', 'animations', 'thinking-anim.png');
  assert.ok(fs.existsSync(apngPath), 'thinking-anim.png must exist');
  const buf = fs.readFileSync(apngPath);
  assert.deepEqual([...buf.subarray(0, 8)], [137, 80, 78, 71, 13, 10, 26, 10], 'thinking-anim.png must be a valid PNG');
  assert.ok(buf.includes(Buffer.from('acTL')), 'thinking-anim.png must contain an APNG acTL animation chunk');
});

test('PDF preview preserves aspect ratio, handles fallback errors, and disposes on Resume unmount', () => {
  const pdfPreview = fs.readFileSync(path.join(rendererDir, 'js', 'pdf-preview.js'), 'utf8');
  const viewsCss = fs.readFileSync(path.join(rendererDir, 'styles', 'views.css'), 'utf8');
  const resumeBuilder = fs.readFileSync(path.join(rendererDir, 'views', 'resume-builder', 'resume-builder.js'), 'utf8');

  assert.ok(!pdfPreview.includes('canvas.style.height'), 'pdf-preview.js must not pin inline canvas height');
  assert.ok(pdfPreview.includes('setDragRegionsEnabled'), 'pdf-preview.js must toggle data-tauri-drag-region on expand');
  assert.match(viewsCss, /\.pdf-page-canvas\s*\{[^}]*height:\s*auto/, 'views.css must set height: auto on .pdf-page-canvas');
  assert.ok(resumeBuilder.includes('pdf.dispose()'), 'resume-builder.js must dispose PDF preview on unmount');
  assert.ok(resumeBuilder.includes('makeMonthPicker'), 'resume-builder.js must include WebKit month+year picker fallback');
});

test('every IPC channel invoked in tauri-bridge.js is handled in Rust routes.rs or commands.rs', () => {
  const bridge = fs.readFileSync(path.join(rendererDir, 'js', 'tauri-bridge.js'), 'utf8');
  const rust = fs.readFileSync(path.join(tauriDir, 'src', 'routes.rs'), 'utf8') +
    '\n' + fs.readFileSync(path.join(tauriDir, 'src', 'commands.rs'), 'utf8');

  const channels = [...new Set([...bridge.matchAll(/call\('([^']+)'/g)].map(m => m[1]))];
  assert.ok(channels.length > 50, 'expected all IPC channels from tauri-bridge.js');
  const missing = channels.filter(ch => !rust.includes(`"${ch}"`));
  assert.deepEqual(missing, [], 'IPC channels in tauri-bridge.js missing from Rust');
});
