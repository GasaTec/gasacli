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
const { normalizeHost, parseChecksum, requestBuffer, tarCommand, verifySha256 } = require('../lib/download');
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

function makeZip(entries) {
  const crc32 = (buffer) => {
    let crc = 0xffffffff;
    for (const byte of buffer) {
      crc ^= byte;
      for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ (0xedb88320 & -(crc & 1));
    }
    return (crc ^ 0xffffffff) >>> 0;
  };
  const locals = []; const centrals = []; let offset = 0;
  for (const { name, data } of entries) {
    const nameBytes = Buffer.from(name); const content = Buffer.from(data); const crc = crc32(content);
    const local = Buffer.alloc(30); local.writeUInt32LE(0x04034b50, 0); local.writeUInt16LE(20, 4); local.writeUInt32LE(crc, 14); local.writeUInt32LE(content.length, 18); local.writeUInt32LE(content.length, 22); local.writeUInt16LE(nameBytes.length, 26);
    locals.push(local, nameBytes, content);
    const central = Buffer.alloc(46); central.writeUInt32LE(0x02014b50, 0); central.writeUInt16LE(20, 4); central.writeUInt16LE(20, 6); central.writeUInt32LE(crc, 16); central.writeUInt32LE(content.length, 20); central.writeUInt32LE(content.length, 24); central.writeUInt16LE(nameBytes.length, 28); central.writeUInt32LE(offset, 42);
    centrals.push(central, nameBytes); offset += local.length + nameBytes.length + content.length;
  }
  const centralSize = centrals.reduce((sum, chunk) => sum + chunk.length, 0); const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0); end.writeUInt16LE(entries.length, 8); end.writeUInt16LE(entries.length, 10); end.writeUInt32LE(centralSize, 12); end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, ...centrals, end]);
}

test('maps supported operating systems and architectures', () => {
  assert.deepEqual(getPlatform('win32', 'x64'), { os: 'windows', arch: 'amd64', extension: 'zip', binary: 'gasa.exe' });
  assert.deepEqual(getPlatform('darwin', 'arm64'), { os: 'darwin', arch: 'arm64', extension: 'tar.gz', binary: 'gasa' });
  assert.deepEqual(getPlatform('linux', 'x64').os, 'linux');
  assert.throws(() => getPlatform('freebsd', 'x64'), /Unsupported platform/);
  assert.equal(normalizeHost('GitHub.com'), 'github.com');
});

test('rejects malformed prerelease identifiers', async () => {
  for (const version of ['1.2.3-.', '1.2.3-..', '1.2.3-alpha.', '1.2.3-alpha..beta']) {
    await assert.rejects(downloadBinary({ version, platform: 'linux', arch: 'x64', fetch: async () => { throw new Error('unexpected download'); } }), /Invalid package release version/);
  }
});

test('parses the matching archive checksum and rejects a missing archive', () => {
  const hash = 'a'.repeat(64);
  assert.equal(parseChecksum(`${hash}  gasa_1.2.3_linux_amd64.tar.gz\n`, 'gasa_1.2.3_linux_amd64.tar.gz'), hash);
  assert.equal(parseChecksum(`${hash.toUpperCase()}  gasa_1.2.3_linux_amd64.tar.gz\n`, 'gasa_1.2.3_linux_amd64.tar.gz'), hash);
  assert.throws(() => parseChecksum(`${hash}\u00a0gasa_1.2.3_linux_amd64.tar.gz\n`, 'gasa_1.2.3_linux_amd64.tar.gz'), /missing/);
  assert.throws(() => parseChecksum(`${hash}\u3000gasa_1.2.3_linux_amd64.tar.gz\n`, 'gasa_1.2.3_linux_amd64.tar.gz'), /missing/);
  assert.throws(() => parseChecksum(`${hash}  gasa_1.2.3_linux_amd64.tar.gz\u0000\n`, 'gasa_1.2.3_linux_amd64.tar.gz'), /missing/);
  assert.throws(() => parseChecksum(`${hash}  other.zip`, 'wanted.zip'), /missing/);
  const name = 'gasa_1.2.3_linux_amd64.tar.gz';
  const other = 'b'.repeat(64);
  assert.throws(() => parseChecksum(`${hash}  ${name}\r\n${other}  ${name}\r\n`, name), /conflicting checksums/);
  assert.throws(() => parseChecksum(`${other}  ${name}\n${hash}  ${name}\n`, name), /conflicting checksums/);
  assert.equal(parseChecksum(`${hash} *${name}\n`, name), hash);
  assert.equal(parseChecksum(`\uFEFF ${hash}\t${name}\r\n`, name), hash);
  assert.throws(() => parseChecksum(`${hash}  ${name}.sig\n`, name), /missing/);
  assert.throws(() => parseChecksum(`${hash}  ${name}.sbom\n`, name), /missing/);
  assert.equal(parseChecksum(`${hash}  ${name}\r\n${hash}  ${name}\r\n`, name), hash);
});

test('matches the shared checksum parser parity fixtures', () => {
  const fixture = fs.readFileSync(path.join(__dirname, 'fixtures/checksum-parity.txt'), 'utf8');
  const archive = 'gasa_1.2.3_linux_amd64.tar.gz';
  for (const row of fixture.trimEnd().split(/\r?\n/)) {
    const [name, expected, encoded] = row.split('|');
    const input = Buffer.from(encoded, 'base64').toString('utf8');
    if (expected === 'CONFLICT') assert.throws(() => parseChecksum(input, archive), /conflicting checksums/, name);
    else if (expected === 'MISSING') assert.throws(() => parseChecksum(input, archive), /missing/, name);
    else assert.equal(parseChecksum(input, archive), expected, name);
  }
});

test('accepts matching SHA-256 and rejects a mismatch', () => {
  const data = Buffer.from('test archive');
  const hash = crypto.createHash('sha256').update(data).digest('hex');
  assert.equal(verifySha256(data, hash), true);
  assert.throws(() => verifySha256(data, '0'.repeat(64)), /mismatch/);
});

test('download idle and total timeouts are classified as network failures', async (t) => {
  const server = http.createServer((_req, _res) => {});
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  t.after(() => server.closeAllConnections());
  t.after(() => server.close());
  const base = `http://127.0.0.1:${server.address().port}`;
  for (const [pathName, options] of [['idle', { idleTimeout: 10, totalTimeout: 1000 }], ['total', { idleTimeout: 1000, totalTimeout: 10 }]]) {
    await assert.rejects(requestBuffer(`${base}/${pathName}`, { ...options, allowedHosts: new Set(['127.0.0.1']), allowHttp: true }), (error) => {
      assert.equal(error.network, true);
      assert.match(error.message, pathName === 'idle' ? /Download idle timeout/ : /Total download timed out/);
      return true;
    });
  }
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
  const truncated = Buffer.from('truncated archive with a valid checksum');
  const truncatedHash = crypto.createHash('sha256').update(truncated).digest('hex');
  await assert.rejects(downloadBinary({ version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir: vendor, releaseBase: 'http://local.invalid', fetch: async (url) => url.endsWith('/checksums.txt') ? Buffer.from(`${truncatedHash}  gasa_1.2.3_linux_amd64.tar.gz\n`) : truncated }), /Could not inspect|Could not extract/);
  assert.equal(fs.existsSync(path.join(vendor, 'gasa')), false);
});

test('downloadBinary rejects symlink and hardlink archive entries', async (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-links-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const executable = Buffer.alloc(2048, 1);
  const symlinkData = makeTarGz([{ name: 'gasa', type: '0', data: executable }, { name: 'link', type: '2', link: 'gasa' }]);
  const symlinkHash = crypto.createHash('sha256').update(symlinkData).digest('hex');
  await assert.rejects(downloadBinary({ version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir: path.join(temp, 'symvendor'), releaseBase: 'http://local.invalid', fetch: async (url) => url.endsWith('/checksums.txt') ? Buffer.from(`${symlinkHash}  gasa_1.2.3_linux_amd64.tar.gz\n`) : symlinkData }), /non-regular entry|symbolic link or hard link|Could not extract/);

  const hardlinkData = makeTarGz([{ name: 'gasa', type: '0', data: executable }, { name: 'hardlink', type: '1', link: 'gasa' }]);
  const hash = crypto.createHash('sha256').update(hardlinkData).digest('hex');
  await assert.rejects(downloadBinary({ version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir: path.join(temp, 'hardvendor'), releaseBase: 'http://local.invalid', fetch: async (url) => url.endsWith('/checksums.txt') ? Buffer.from(`${hash}  gasa_1.2.3_linux_amd64.tar.gz\n`) : hardlinkData }), /non-regular entry|symbolic link or hard link|not a regular executable/);
});

test('downloadBinary validates every tar entry before extraction', async (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-tar-policy-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const executable = Buffer.alloc(2048, 3);
  const cases = [
    ['case mismatch', [{ name: 'GASA', type: '0', data: executable }]],
    ['duplicate binary', [{ name: 'gasa', type: '0', data: executable }, { name: 'gasa', type: '0', data: executable }]],
    ['directory entry', [{ name: 'gasa/', type: '5' }, { name: 'gasa/file', type: '0', data: executable }]],
    ['dot prefix duplicate', [{ name: './gasa', type: '0', data: executable }, { name: 'gasa', type: '0', data: executable }]],
    ['symlink directory', [{ name: 'gasa', type: '0', data: executable }, { name: 'evil', type: '2', link: '/tmp' }, { name: 'evil/file', type: '0', data: executable }]],
    ['hard link', [{ name: 'gasa', type: '0', data: executable }, { name: 'LICENSE', type: '1', link: 'gasa' }]],
    ['parent traversal', [{ name: 'x/../gasa', type: '0', data: executable }]],
    ['absolute path', [{ name: '/tmp/gasa', type: '0', data: executable }]],
    ['unexpected extra', [{ name: 'gasa', type: '0', data: executable }, { name: 'script.sh', type: '0', data: Buffer.from('bad') }]],
  ];
  for (const [label, entries] of cases) {
    const archive = makeTarGz(entries);
    const hash = crypto.createHash('sha256').update(archive).digest('hex');
    const vendorDir = path.join(temp, label.replaceAll(' ', '-'));
    await assert.rejects(downloadBinary({
      version: '1.2.3', platform: 'linux', arch: 'x64', vendorDir, releaseBase: 'http://local.invalid',
      fetch: async (url) => url.endsWith('/checksums.txt') ? Buffer.from(`${hash}  gasa_1.2.3_linux_amd64.tar.gz\n`) : archive,
    }), /Release archive|Could not inspect/);
    assert.equal(fs.existsSync(vendorDir), false, `${label} must be rejected before installing`);
  }
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
  const invalidFixtures = [
    ['duplicate root executable', [{ name: 'gasa.exe', data: Buffer.alloc(2048, 1) }, { name: 'gasa.exe', data: Buffer.alloc(2048, 2) }]],
    ['case-mismatched executable', [{ name: 'GASA.EXE', data: Buffer.alloc(2048, 1) }]],
    ['nested executable', [{ name: 'bin/gasa.exe', data: Buffer.alloc(2048, 1) }]],
  ];
  for (const [label, entries] of invalidFixtures) {
    const invalidArchive = makeZip(entries);
    await assert.rejects(downloadBinary({
      version: '1.2.3', platform: 'win32', arch: 'x64', vendorDir: path.join(temp, label.replaceAll(' ', '-')),
      releaseBase: 'http://local.invalid',
      fetch: async (url) => url.endsWith('/checksums.txt') ? Buffer.from(`${crypto.createHash('sha256').update(invalidArchive).digest('hex')}  gasa_1.2.3_windows_amd64.zip\n`) : invalidArchive,
    }), /exactly one regular gasa\.exe at its root|unexpected entry/);
  }
});

test('postinstall skips network errors but fails loudly on checksum errors', (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-postinstall-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const preload = path.join(temp, 'mock-download.js');
  fs.writeFileSync(preload, [
    "const Module = require('node:module');",
    'const load = Module._load;',
    "Module._load = function(request, parent, isMain) { if (request === '../lib/download' && parent.filename.endsWith('postinstall.js')) return { downloadBinary: () => { const e = new Error(process.env.GASA_POSTINSTALL_ERROR); if (process.env.GASA_POSTINSTALL_CODE) e.code = process.env.GASA_POSTINSTALL_CODE; if (process.env.GASA_POSTINSTALL_STATUS) e.statusCode = Number(process.env.GASA_POSTINSTALL_STATUS); if (e.statusCode === 404) e.releaseNotReady = true; if (e.statusCode >= 500 || e.code) e.network = true; return Promise.reject(e); } }; return load.call(this, request, parent, isMain); };",
  ].join('\n'));
  const script = path.resolve(__dirname, '../scripts/postinstall.js');
  const network = spawnSync(process.execPath, ['-r', preload, script], { encoding: 'utf8', env: { ...process.env, GASA_POSTINSTALL_ERROR: 'HTTP failure', GASA_POSTINSTALL_STATUS: '503' } });
  assert.equal(network.status, 0, network.stderr);
  assert.match(network.stderr, /binary download skipped/);
  const integrity = spawnSync(process.execPath, ['-r', preload, script], { encoding: 'utf8', env: { ...process.env, GASA_POSTINSTALL_ERROR: 'SHA-256 mismatch; archive not installed' } });
  assert.notEqual(integrity.status, 0);
  assert.match(integrity.stderr, /integrity check failed, binary NOT installed/);
  for (const failure of ['conflicting checksums for archive', 'Could not extract release archive with tar', 'Release archive entry gasa is not a regular executable file']) {
    const result = spawnSync(process.execPath, ['-r', preload, script], { encoding: 'utf8', env: { ...process.env, GASA_POSTINSTALL_ERROR: failure } });
    assert.notEqual(result.status, 0, `${failure} must fail postinstall`);
  }
  for (const [failure, code] of [['name resolution', 'ENOTFOUND'], ['reset', 'ECONNRESET'], ['timeout', 'ETIMEDOUT'], ['refused', 'ECONNREFUSED'], ['retry', 'EAI_AGAIN']]) {
    const result = spawnSync(process.execPath, ['-r', preload, script], { encoding: 'utf8', env: { ...process.env, GASA_POSTINSTALL_ERROR: failure, GASA_POSTINSTALL_CODE: code } });
    assert.equal(result.status, 0, `${failure} should leave install usable`);
    assert.match(result.stderr, /retry the download the next time it is run/);
  }
  const notReady = spawnSync(process.execPath, ['-r', preload, script], { encoding: 'utf8', env: { ...process.env, GASA_POSTINSTALL_ERROR: 'HTTP 404', GASA_POSTINSTALL_STATUS: '404' } });
  assert.equal(notReady.status, 0, notReady.stderr);
  const falsePositive = spawnSync(process.execPath, ['-r', preload, script], { encoding: 'utf8', env: { ...process.env, GASA_POSTINSTALL_ERROR: 'tar stderr mentions ECONNRESET' } });
  assert.notEqual(falsePositive.status, 0);

  const cliRoot = path.join(temp, 'cli');
  fs.mkdirSync(path.join(cliRoot, 'bin'), { recursive: true }); fs.mkdirSync(path.join(cliRoot, 'lib')); fs.mkdirSync(path.join(cliRoot, 'vendor'));
  fs.copyFileSync(path.resolve(__dirname, '../bin/gasa.js'), path.join(cliRoot, 'bin/gasa.js'));
  fs.copyFileSync(path.resolve(__dirname, '../lib/platform.js'), path.join(cliRoot, 'lib/platform.js'));
  fs.writeFileSync(path.join(cliRoot, 'vendor/gasa'), 'corrupt binary');
  const recoveryPreload = path.join(temp, 'mock-recovery.js');
  fs.writeFileSync(recoveryPreload, [
    "const Module=require('node:module'),fs=require('node:fs');const load=Module._load;",
    "Module._load=function(request,parent,isMain){if(request==='../lib/download'&&parent.filename.replace(/\\\\/g,'/').endsWith('bin/gasa.js'))return{downloadBinary:async()=>{fs.mkdirSync(require('node:path').dirname(process.env.GASA_RECOVERY_BINARY),{recursive:true});fs.copyFileSync(process.execPath,process.env.GASA_RECOVERY_BINARY);}};return load.call(this,request,parent,isMain);};",
  ].join('\n'));
  const recovered = spawnSync(process.execPath, ['-r', recoveryPreload, path.join(cliRoot, 'bin/gasa.js')], { encoding: 'utf8', env: { ...process.env, GASA_RECOVERY_BINARY: path.join(cliRoot, 'vendor', getPlatform().binary) } });
  assert.equal(recovered.status, 0, recovered.stderr);
});

test('downloadBinary writes the destination by atomic rename and cleans staged files on rename failure', async (t) => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gasacli-atomic-'));
  t.after(() => fs.rmSync(temp, { recursive: true, force: true }));
  const source = path.join(temp, 'source'); fs.mkdirSync(source);
  fs.writeFileSync(path.join(source, 'gasa'), Buffer.alloc(2048, 7));
  fs.writeFileSync(path.join(source, 'LICENSE'), 'license');
  fs.writeFileSync(path.join(source, 'README.md'), 'readme');
  const archivePath = path.join(temp, 'archive.tar.gz');
  const packed = spawnSync(tarCommand(), ['-czf', archivePath, '-C', source, 'gasa', 'LICENSE', 'README.md']);
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
