import assert from 'node:assert/strict';
import { readFile, stat } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { resolve, sep } from 'node:path';

const root = fileURLToPath(new URL('../public/', import.meta.url));
const html = await readFile(resolve(root, 'index.html'), 'utf8');
const css = (await Promise.all(['style.css', 'demo.css'].map((file) => readFile(resolve(root, file), 'utf8')))).join('\n');
const js = (await Promise.all(['site.js', 'demo.js', 'demo-playback.js'].map((file) => readFile(resolve(root, file), 'utf8')))).join('\n');
const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1]);
assert.equal(new Set(ids).size, ids.length, 'IDs must be unique');
assert.equal((html.match(/<h1\b/g) || []).length, 1, 'Exactly one page heading');
assert.match(html, /<html lang="zh-CN">/);
for (const match of html.matchAll(/\b(?:href|src|aria-controls|aria-labelledby)="([^"]+)"/g)) {
  const value = match[1];
  if (match[0].startsWith('aria-')) {
    for (const id of value.split(' ')) assert(ids.includes(id), `Missing accessibility target: ${id}`);
    continue;
  }
  if (value.startsWith('#')) {
    assert(ids.includes(value.slice(1)), `Missing anchor: ${value}`);
  } else if (!/^[a-z]+:/i.test(value)) {
    await stat(resolve(root, value));
  } else {
    assert(/^(https:|mailto:)/.test(value), `Unsupported external URL: ${value}`);
  }
}
for (const image of html.matchAll(/<img\b[^>]*>/g)) assert.match(image[0], /\balt="[^"]*"/, 'Images need alt text');
for (const match of css.matchAll(/url\(['"]?([^'"()]+)['"]?\)/g)) {
  if (!match[1].startsWith('#')) await stat(resolve(root, match[1]));
}
assert.match(css, /prefers-reduced-motion/);
assert.match(js, /reduceMotion\.matches/);
const releases = JSON.parse(await readFile(resolve(root, 'releases.json'), 'utf8'));
for (const platform of ['android', 'ios']) {
  const release = releases[platform];
  assert.equal(typeof release.available, 'boolean', `Missing ${platform} availability`);
  if (release.available) {
    assert.equal(typeof release.version, 'string');
    assert(release.version.trim());
    const url = new URL(release.url);
    assert.equal(url.protocol, 'https:');
    assert(!url.username && !url.password, 'Public download URLs cannot contain credentials');
    if (platform === 'android') {
      assert.equal(release.url, 'https://www.speechful.dpdns.org/downloads/SpeechFUL.apk');
      assert.equal(release.qr, 'assets/android-download.png');
      await stat(resolve(root, release.qr));
      assert(html.includes('href="' + release.url + '"'), 'Published download must exist without JavaScript');
      assert(html.includes('<p id="android-description">v' + release.version + '</p>'));
      assert(html.includes('src="' + release.qr + '"'));
    }
  } else {
    assert.equal(release.url, null, 'Unpublished platform must not expose an installer URL');
  }
  if (!release.available) assert.match(html, new RegExp(`<span class="release-status" id="${platform}-action">暂未开放`));
}
assert.equal((await readFile(resolve(root, '_redirects'), 'utf8')).trim(), `/downloads/SpeechFUL.apk https://gitee.com/axibass/speechful-android-release/releases/download/v${encodeURIComponent(releases.android.version)}/SpeechFUL.apk 302`);
// Prevent external business calls: this site only reads its local release manifest.
assert(!/fetch\s*\(\s*['"]https?:/.test(js), 'Website must not fetch live business APIs');
assert.equal((js.match(/\bfetch\s*\(/g) || []).length, 0, 'Downloads must not depend on a runtime request');
assert(!/\b(?:XMLHttpRequest|WebSocket|sendBeacon)\b/.test(js), 'The demonstration must remain offline');
for (const stage of ['welcome', 'feed', 'comments', 'messages']) {
  assert.match(html, new RegExp(`data-screen="${stage}"`), `Missing demo stage: ${stage}`);
}
assert.match(html, /aria-label="回到顶部"/);
assert(!/海南大学|海大|Hainan University/i.test(html + js), 'Public copy must not name the institution');
assert(resolve(root, 'index.html').startsWith(resolve(root) + sep));
console.log('PASS: local assets, navigation, accessibility references, release manifest, static fallback, reduced motion and content boundaries.');
