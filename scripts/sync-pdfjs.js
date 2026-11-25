#!/usr/bin/env node
// Copies pdfjs-dist browser ES modules into app/renderer/vendor/pdfjs/ so the renderer can
// draw PDF previews to <canvas> identically across WKWebView, WebView2, and WebKitGTK.
const fs = require('node:fs');
const path = require('node:path');

const root = path.join(__dirname, '..');
const srcDir = path.join(root, 'node_modules', 'pdfjs-dist', 'build');
const outDir = path.join(root, 'app', 'renderer', 'vendor', 'pdfjs');
const files = ['pdf.min.mjs', 'pdf.worker.min.mjs'];

if (!fs.existsSync(srcDir)) {
  process.exit(0);
}

fs.mkdirSync(outDir, { recursive: true });
for (const file of files) {
  const from = path.join(srcDir, file);
  if (fs.existsSync(from)) {
    fs.copyFileSync(from, path.join(outDir, file));
  }
}
