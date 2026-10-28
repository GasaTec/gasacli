'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');
const zlib = require('node:zlib');
const { spawnSync } = require('node:child_process');
const { getPlatform } = require('../lib/platform');
const { parseChecksum, requestBuffer, tarCommand, verifySha256 } = require('../lib/download');
const { handleChildExit, installSignalForwarding } = require('../bin/gasa');
const { EventEmitter } = require('node:events');
const { downloadBinary } = require('../lib/download');

function makeTarGz(entries) {
  const chunks = [];
  for (const entry of entries) {
    const header = Buffer.alloc(512);
    const write = (value, offset, length) => header.write(String(value), offset, length, 'utf8');
    const octal = (value, length) => value.toString(8).padStart(length - 1, '0') + '\0';
    write(entry.name, 0, 100); write(octal(0o644, 8), 100, 8); write(octal(0, 8), 108, 8); write(octal(0, 8), 116, 8);
    write(octal(entry.data?.length || 0, 12), 124, 12); write(octal(0, 12), 136, 12); header.fill(32, 148, 156);
    write(entry.type, 156, 1); if (entry.link) write(entry.link, 157, 100); write('ustar\0', 257, 6); write('00', 263, 2);
    const sum = header.reduce((total, byte) => total + byte, 0); write(sum.toString(8).padStart(6, '0') + '\0 ', 148, 8);
    chunks.push(header);
    if (entry.data?.length) { chunks.push(entry.data); const padding = (512 - entry.data.length % 512) % 512; if (padding) chunks.push(Buffer.alloc(padding)); }
  }
  chunks.push(Buffer.alloc(1024));
  return zlib.gzipSync(Buffer.concat(chunks));
}

test('maps supported operating systems and architectures', () => {
  assert.deepEqual(getPlatform('win32', 'x64'), { os: 'windows', arch: 'amd64', extension: 'zip', binary: 'gasa.exe' });
  assert.deepEqual(getPlatform('darwin', 'arm64'), { os: 'darwin', arch: 'arm64', extension: 'tar.gz', binary: 'gasa' });
  assert.deepEqual(getPlatform('linux', 'x64').os, 'linux');
  assert.throws(() => getPlatform('freebsd', 'x64'), /Unsupported platform/);
});

test('parses the matching archive checksum and rejects a missing archive', () => {
  const hash = 'a'.repeat(64);
  assert.equal(parseChecksum(`${hash}  gasa_1.2.3_linux_amd64.tar.gz\n`, 'gasa_1.2.3_linux_amd64.tar.gz'), hash);
  assert.throws(() => parseChecksum(`${hash}  other.zip`, 'wanted.zip'), /missing/);
});

test('accepts matching SHA-256 and rejects a mismatch', () => {
  const data = Buffer.from('test archive');
  const hash = crypto.createHash('sha256').update(data).digest('hex');
  assert.equal(verifySha256(data, hash), true);
  assert.throws(() => verifySha256(data, '0'.repeat(64)), /mismatch/);
});

test('downloadBinary stops after a bad checksum and installs nothing', async (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-checksum-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const requests = [];
  const server = http.createServer((req, res) => { requests.push(req.url); res.writeHead(200); res.end('bad checksum'); });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  t.after(() => server.close());
  const started = Date.now();
  await assert.rejects(downloadBinary({
    version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir: path.join(temp, 'vendor'),
    releaseBase: `http://127.0.0.1:${server.address().port}`,
    fetch: async (url) => { const response = await fetch(url); return Buffer.from(await response.arrayBuffer()); },
  }), /missing from checksums/);
  assert.deepEqual(requests, ['/v1.2.3/checksums.txt']);
  assert.ok(Date.now() - started < 2000, 'checksum failure should stop before the archive request');
  assert.equal(fs.existsSync(path.join(temp, 'vendor', 'gasa')), false);
});

test('downloadBinary rejects a local archive with a checksum mismatch without installing it', async (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-mismatch-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const archive = Buffer.from('archive bytes with a different digest');
  const wrongHash = crypto.createHash('sha256').update('not this archive').digest('hex');
  const server = http.createServer((req, res) => {
    res.writeHead(200);
    res.end(req.url.endsWith('/checksums.txt') ? `${wrongHash}  gasa_1.2.3_linux_amd64.tar.gz\n` : archive);
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  t.after(() => server.close());
  const vendor = path.join(temp, 'vendor');
  await assert.rejects(downloadBinary({ version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir: vendor, releaseBase: `http://127.0.0.1:${server.address().port}`, fetch: async (url) => { const response = await fetch(url); return Buffer.from(await response.arrayBuffer()); } }), /SHA-256 mismatch/);
  assert.equal(fs.existsSync(path.join(vendor, 'gasa')), false);
});

test('downloadBinary rejects symlink and hardlink archive entries', async (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-links-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const executable = Buffer.alloc(2048, 1);
  const symlinkData = makeTarGz([{ name: 'gasa', type: '0', data: executable }, { name: 'link', type: '2', link: 'gasa' }]);
  const symlinkHash = crypto.createHash('sha256').update(symlinkData).digest('hex');
  await assert.rejects(downloadBinary({ version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir: path.join(temp, 'symvendor'), releaseBase: 'http://local.invalid', fetch: async (url) => url.endsWith('/checksums.txt') ? Buffer.from(`${symlinkHash}  gasa_1.2.3_linux_amd64.tar.gz\n`) : symlinkData }), /symbolic link or hard link|Could not extract/);

  const hardlinkData = makeTarGz([{ name: 'gasa', type: '0', data: executable }, { name: 'hardlink', type: '1', link: 'gasa' }]);
  const hash = crypto.createHash('sha256').update(hardlinkData).digest('hex');
  await assert.rejects(downloadBinary({ version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir: path.join(temp, 'hardvendor'), releaseBase: 'http://local.invalid', fetch: async (url) => url.endsWith('/checksums.txt') ? Buffer.from(`${hash}  gasa_1.2.3_linux_amd64.tar.gz\n`) : hardlinkData }), /symbolic link or hard link|not a regular executable/);
});

test('downloadBinary rejects an extracted tar bomb larger than 100 MiB', async (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-tarbomb-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const archive = makeTarGz([{ name: 'gasa', type: '0', data: Buffer.alloc(100 * 1024 * 1024 + 1) }]);
  const hash = crypto.createHash('sha256').update(archive).digest('hex');
  const vendor = path.join(temp, 'vendor');
  await assert.rejects(downloadBinary({
    version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir: vendor,
    releaseBase: 'http://local.invalid',
    fetch: async (url) => url.endsWith('/checksums.txt') ? Buffer.from(`${hash}  gasa_1.2.3_linux_amd64.tar.gz\n`) : archive,
  }), /100 MiB size limit/);
  assert.equal(fs.existsSync(path.join(vendor, 'gasa')), false);
});

test('downloadBinary extracts a generated ZIP fixture on Windows', { skip: process.platform !== 'win32' }, async (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-zip-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const source = path.join(temp, 'source'); fs.mkdirSync(source);
  fs.writeFileSync(path.join(source, 'gasa.exe'), Buffer.alloc(2048, 7));
  const archivePath = path.join(temp, 'fixture.zip');
  const packed = spawnSync(tarCommand(), ['-a', '-cf', archivePath, '-C', source, 'gasa.exe'], { encoding: 'utf8' });
  assert.equal(packed.status, 0, `ZIP fixture failed: ${packed.stderr}`);
  const archive = fs.readFileSync(archivePath); const hash = crypto.createHash('sha256').update(archive).digest('hex');
  const vendorDir = path.join(temp, 'vendor');
  const installed = await downloadBinary({
    version: '1.2.3', platform: 'win32', arch: 'x64', vendorDir, releaseBase: 'http://local.invalid',
    fetch: async (url) => url.endsWith('/checksums.txt') ? Buffer.from(`${hash}  gasa_1.2.3_windows_amd64.zip\n`) : archive,
  });
  assert.equal(installed, path.join(vendorDir, 'gasa.exe'));
  assert.equal(fs.readFileSync(installed).length, 2048);
});

test('postinstall skips network errors but fails loudly on checksum errors', (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-postinstall-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const preload = path.join(temp, 'mock-download.js');
  fs.writeFileSync(preload, [
    "const Module = require('node:module');",
    'const load = Module._load;',
    "Module._load = function(request, parent, isMain) { if (request === '../lib/download' && parent.filename.endsWith('postinstall.js')) return { downloadBinary: () => Promise.reject(new Error(process.env.GASA_POSTINSTALL_ERROR)) }; return load.call(this, request, parent, isMain); };",
  ].join('\n'));
  const script = path.resolve(__dirname, '../scripts/postinstall.js');
  const network = spawnSync(process.execPath, ['-r', preload, script], { encoding: 'utf8', env: { ...process.env, GASA_POSTINSTALL_ERROR: 'Download failed with HTTP 503' } });
  assert.equal(network.status, 0, network.stderr);
  assert.match(network.stderr, /binary download skipped/);
  const integrity = spawnSync(process.execPath, ['-r', preload, script], { encoding: 'utf8', env: { ...process.env, GASA_POSTINSTALL_ERROR: 'SHA-256 mismatch; archive not installed' } });
  assert.notEqual(integrity.status, 0);
  assert.match(integrity.stderr, /integrity check failed, binary NOT installed/);
});

test('downloadBinary writes the destination by atomic rename and cleans staged files on rename failure', async (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-atomic-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const source = path.join(temp, 'source'); fs.mkdirSync(source);
  fs.writeFileSync(path.join(source, 'gasa'), Buffer.alloc(2048, 7));
  const archivePath = path.join(temp, 'archive.tar.gz');
  const packed = spawnSync(tarCommand(), ['-czf', archivePath, '-C', source, 'gasa']);
  assert.equal(packed.status, 0, `tar fixture failed: ${packed.stderr}`);
  const archive = fs.readFileSync(archivePath); const hash = crypto.createHash('sha256').update(archive).digest('hex');
  const fetch = async (url) => url.endsWith('/checksums.txt') ? Buffer.from(`${hash}  gasa_1.2.3_linux_amd64.tar.gz\n`) : archive;
  const vendor = path.join(temp, 'vendor');
  const installed = await downloadBinary({ version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir: vendor, releaseBase: 'http://local.invalid', fetch });
  assert.equal(installed, path.join(vendor, 'gasa'));
  assert.equal(fs.readFileSync(installed).length, 2048);
  assert.equal(fs.readdirSync(vendor).some((name) => name.endsWith('.tmp')), false);
  fs.writeFileSync(installed, 'previous');
  const originalRename = fs.renameSync;
  fs.renameSync = () => { throw new Error('forced rename failure'); };
  try { await assert.rejects(downloadBinary({ version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir: vendor, releaseBase: 'http://local.invalid', fetch }), /forced rename failure/); }
  finally { fs.renameSync = originalRename; }
  assert.equal(fs.readFileSync(installed, 'utf8'), 'previous');
  assert.deepEqual(fs.readdirSync(vendor), ['gasa']);
});

test('downloads a locally served generated archive and enforces redirect hosts', async (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-test-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const payload = Buffer.from('test executable');
  const archivePath = path.join(temp, 'gasa.tar.gz');
  fs.writeFileSync(path.join(temp, 'gasa'), payload);
  const tar = spawnSync(tarCommand(), ['-czf', archivePath, '-C', temp, 'gasa']);
  assert.equal(tar.status, 0, `tar failed: ${tar.stderr}`);
  const archive = fs.readFileSync(archivePath);
  const checksum = crypto.createHash('sha256').update(archive).digest('hex');
  let server;
  server = http.createServer((req, res) => {
    if (req.url === '/archive') { res.writeHead(200); res.end(archive); }
    else if (req.url === '/large') { res.writeHead(200); res.end(Buffer.alloc(32)); }
    else if (req.url === '/redirect') { res.writeHead(302, { location: 'http://example.invalid/archive' }); res.end(); }
    else { res.writeHead(404); res.end(); }
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  t.after(() => server.close());
  const base = `http://127.0.0.1:${server.address().port}`;
  const localOptions = { allowedHosts: new Set(['127.0.0.1']), allowHttp: true };
  const downloaded = await requestBuffer(`${base}/archive`, localOptions);
  assert.deepEqual(downloaded, archive);
  assert.equal(verifySha256(downloaded, checksum), true);
  await assert.rejects(requestBuffer(`${base}/redirect`, localOptions), /redirect host is not allowed/);
  await assert.rejects(requestBuffer(`${base}/large`, { ...localOptions, maxBytes: 16 }), /16 byte size limit/);
  await assert.rejects(requestBuffer('https://127.0.0.1:8443/archive', { allowedHosts: new Set(['127.0.0.1']) }), /Download host is not allowed/);
});

test('removes signal forwarding listeners before child signal propagation', () => {
  const child = new EventEmitter();
  child.killed = false;
  child.kill = () => { child.killed = true; };
  const fakeProcess = new EventEmitter();
  fakeProcess.pid = 123;
  fakeProcess.kill = (pid, signal) => {
    assert.equal(pid, 123);
    assert.equal(signal, 'SIGTERM');
    assert.equal(fakeProcess.listenerCount('SIGTERM'), 0);
  };
  const cleanup = installSignalForwarding(child, fakeProcess);
  assert.equal(fakeProcess.listenerCount('SIGTERM'), 1);
  handleChildExit(fakeProcess, cleanup, null, 'SIGTERM');
  assert.equal(fakeProcess.listenerCount('SIGTERM'), 0);
});
