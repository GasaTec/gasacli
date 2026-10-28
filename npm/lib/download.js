'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const https = require('node:https');
const { getPlatform } = require('./platform');

const RELEASE_BASE = 'https://github.com/GasaTec/gasacli/releases/download';
const ALLOWED_HOSTS = new Set(['github.com', 'objects.githubusercontent.com', 'release-assets.githubusercontent.com']);
const TIMEOUT_MS = 120_000;
const IDLE_TIMEOUT_MS = 30_000;
const MAX_DOWNLOAD_BYTES = 200 * 1024 * 1024;
const MAX_EXTRACTED_BINARY_BYTES = 100 * 1024 * 1024;
const MIN_BINARY_BYTES = 1024;

function normalizeHost(host) { return String(host).toLowerCase(); }

function parseChecksum(text, archiveName) {
  const matches = [];
  const normalized = String(text).replace(/^\uFEFF/, '');
  for (const line of normalized.split(/\r?\n/)) {
    const match = /^[\t\n\v\f\r ]*([a-f\d]{64})[\t\n\v\f\r ]+\*?([^\t\n\v\f\r ]+)[\t\n\v\f\r ]*$/i.exec(line);
    if (match && match[2] === archiveName) matches.push(match[1].toLowerCase());
  }
  if (new Set(matches).size > 1) throw new Error(`conflicting checksums for ${archiveName}`);
  if (matches.length) return matches[0];
  throw new Error(`Archive ${archiveName} is missing from checksums.txt`);
}

function verifySha256(buffer, expected) {
  if (!/^[a-f\d]{64}$/i.test(expected)) throw new Error('Invalid SHA-256 checksum in checksums.txt');
  const actual = crypto.createHash('sha256').update(buffer).digest('hex');
  if (actual !== expected.toLowerCase()) throw new Error('SHA-256 mismatch; the archive was not installed');
  return true;
}

function requestBuffer(url, options = {}, redirects = 0, startedAt = Date.now()) {
  const parsed = new URL(url);
  const allowedHosts = options.allowedHosts || ALLOWED_HOSTS;
  const allowed = new Set([...allowedHosts].map(normalizeHost));
  const protocolAllowed = parsed.protocol === 'https:' || (options.allowHttp && redirects === 0 && parsed.protocol === 'http:');
  const portAllowed = !parsed.port || parsed.port === '443' || (options.allowHttp && redirects === 0 && parsed.protocol === 'http:');
  if (!protocolAllowed || !portAllowed || !allowed.has(normalizeHost(parsed.hostname))) {
    return Promise.reject(new Error(`Download host is not allowed: ${parsed.hostname}`));
  }
  const request = parsed.protocol === 'https:' ? https : require('node:http');
  return new Promise((resolve, reject) => {
    const remaining = (options.totalTimeout || TIMEOUT_MS) - (Date.now() - startedAt);
    if (remaining <= 0) return reject(new Error('Total download timed out'));
    let bytes = 0;
    let settled = false;
    const totalTimer = setTimeout(() => req.destroy(new Error('Total download timed out')), remaining);
    const finish = (fn, value) => {
      if (settled) return;
      settled = true;
      clearTimeout(totalTimer);
      if (fn === reject) markNetworkError(value);
      fn(value);
    };
    const req = request.get(parsed, { headers: { 'User-Agent': 'gasacli-npm-installer' } }, (res) => {
      if ([301, 302, 303, 307, 308].includes(res.statusCode)) {
        res.resume();
        if (redirects >= 5) return finish(reject, new Error('Too many redirects while downloading release asset'));
        const location = res.headers.location;
        if (!location) return finish(reject, new Error('Redirect response did not include a location'));
        const next = new URL(location, parsed);
        const nextProtocolAllowed = next.protocol === 'https:';
        const nextPortAllowed = !next.port || next.port === '443';
        if (!allowed.has(normalizeHost(next.hostname)) || !nextProtocolAllowed || !nextPortAllowed) {
          return finish(reject, new Error(`Download redirect host is not allowed: ${next.hostname}`));
        }
        return finish(resolve, requestBuffer(next.href, options, redirects + 1, startedAt));
      }
      if (res.statusCode !== 200) {
        res.resume();
        const error = new Error(`Download failed with HTTP ${res.statusCode}`);
        error.statusCode = res.statusCode;
        return finish(reject, error);
      }
      const chunks = [];
      res.on('data', (chunk) => {
        bytes += chunk.length;
        if (bytes > (options.maxBytes || MAX_DOWNLOAD_BYTES)) {
          res.destroy(new Error(`Download exceeded the ${options.maxBytes || MAX_DOWNLOAD_BYTES} byte size limit`));
          return;
        }
        if (options.budget) {
          options.budget.bytes += chunk.length;
          if (options.budget.bytes > (options.maxBytes || MAX_DOWNLOAD_BYTES)) {
            res.destroy(new Error(`Total downloads exceeded the ${options.maxBytes || MAX_DOWNLOAD_BYTES} byte size limit`));
            return;
          }
        }
        chunks.push(chunk);
      });
      res.on('end', () => finish(resolve, Buffer.concat(chunks)));
      res.on('error', (error) => finish(reject, error));
    });
    req.setTimeout(options.idleTimeout || IDLE_TIMEOUT_MS, () => req.destroy(new Error('Download idle timeout')));
    req.on('error', (error) => finish(reject, error));
  });
}

function packageVersion() {
  return require('../package.json').version;
}

async function getResource(url, options, budget) {
  if (!options.fetch) return requestBuffer(url, { budget });
  const value = await options.fetch(url);
  const buffer = Buffer.isBuffer(value) ? value : Buffer.from(value);
  if (buffer.length > MAX_DOWNLOAD_BYTES) throw new Error(`Download exceeded the ${MAX_DOWNLOAD_BYTES} byte size limit`);
  budget.bytes += buffer.length;
  if (budget.bytes > MAX_DOWNLOAD_BYTES) throw new Error(`Total downloads exceeded the ${MAX_DOWNLOAD_BYTES} byte size limit`);
  return buffer;
}

async function downloadBinary(options = {}) {
  const version = options.version || packageVersion();
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z]+(?:\.[0-9A-Za-z]+)*)?$/.test(version)) throw new Error(`Invalid package release version: ${version}`);
  const target = options.vendorDir || path.resolve(__dirname, '..', 'vendor');
  const { os: platform, arch, extension, binary } = getPlatform(options.platform, options.arch);
  const archiveName = `gasa_${version}_${platform}_${arch}.${extension}`;
  const releaseBase = options.releaseBase || RELEASE_BASE;
  const prefix = `${releaseBase}/v${version}`;
  const checksumUrl = `${prefix}/checksums.txt`;
  const archiveUrl = `${prefix}/${archiveName}`;
  const budget = { bytes: 0 };
  let checksumsBuffer;
  try { checksumsBuffer = await getResource(checksumUrl, options, budget); } catch (error) { markNetworkError(error); throw error; }
  const expectedHash = parseChecksum(checksumsBuffer.toString('utf8'), archiveName);
  let archive;
  try { archive = await getResource(archiveUrl, options, budget); } catch (error) { markNetworkError(error); throw error; }
  verifySha256(archive, expectedHash);

  const tempRoot = fs.mkdtempSync(path.join(options.tempDir || os.tmpdir(), 'gasacli-'));
  try {
    const archivePath = path.join(tempRoot, archiveName);
    const extractedPath = path.join(tempRoot, 'extract');
    fs.writeFileSync(archivePath, archive, { mode: 0o600 });
    fs.mkdirSync(extractedPath);
    const namesResult = spawnSync(tarCommand(), ['-tf', archivePath], { encoding: 'utf8', timeout: TIMEOUT_MS });
    if (namesResult.error || namesResult.status !== 0) throw new Error(`Could not inspect release archive names: ${namesResult.error ? namesResult.error.message : namesResult.stderr}`);
    const typesResult = spawnSync(tarCommand(), ['-tvf', archivePath], { encoding: 'utf8', timeout: TIMEOUT_MS });
    if (typesResult.error || typesResult.status !== 0) throw new Error(`Could not inspect release archive types: ${typesResult.error ? typesResult.error.message : typesResult.stderr}`);
    const names = namesResult.stdout.split(/\r?\n/).filter(Boolean);
    const rows = typesResult.stdout.split(/\r?\n/).filter(Boolean);
    if (names.length !== rows.length) throw new Error('Release archive contains ambiguous names');
    const expected = platform === 'windows' ? 'gasa.exe' : 'gasa';
    let binaryCount = 0;
    for (let index = 0; index < names.length; index++) {
      const name = names[index];
      const row = rows[index];
      const type = row[0];
      if (type !== '-') throw new Error(`Release archive contains a non-regular entry: ${name}`);
      if (name === expected) { binaryCount++; continue; }
      if (name === 'LICENSE' || name === 'README.md') continue;
      throw new Error(`Release archive contains an unexpected entry: ${name}`);
    }
    if (binaryCount !== 1) throw new Error(`Release archive must contain exactly one regular ${expected} at its root`);
    const result = spawnSync(tarCommand(), ['-xf', archivePath, '-C', extractedPath], { encoding: 'utf8', timeout: TIMEOUT_MS });
    if (result.error || result.status !== 0) throw new Error(`Could not extract release archive with tar: ${result.error ? result.error.message : result.stderr}`);
    const inspectEntries = (directory) => {
      for (const name of fs.readdirSync(directory)) {
        const entryPath = path.join(directory, name);
        const entryStat = fs.lstatSync(entryPath);
        if (entryStat.isSymbolicLink() || entryStat.nlink > 1) throw new Error('Release archive contains a symbolic link or hard link');
        if (entryStat.isDirectory()) inspectEntries(entryPath);
      }
    };
    inspectEntries(extractedPath);
    const extractedBinary = path.join(extractedPath, binary);
    let extractedStat;
    try { extractedStat = fs.lstatSync(extractedBinary); } catch { throw new Error(`Release archive does not contain ${binary} at its root`); }
    if (!extractedStat.isFile() || extractedStat.nlink !== 1 || extractedStat.size < MIN_BINARY_BYTES) throw new Error(`Release archive entry ${binary} is not a regular executable file or is too small`);
    if (extractedStat.size > MAX_EXTRACTED_BINARY_BYTES) throw new Error('Extracted executable exceeds the 100 MiB size limit');
    fs.mkdirSync(target, { recursive: true });
    const destination = path.join(target, binary);
    const staged = path.join(target, `.${binary}.${process.pid}.${crypto.randomBytes(8).toString('hex')}.tmp`);
    try {
      fs.copyFileSync(extractedBinary, staged, fs.constants.COPYFILE_EXCL);
      if (platform !== 'windows') fs.chmodSync(staged, 0o755);
      fs.renameSync(staged, destination);
    } catch (error) {
      throw error;
    } finally {
      try { fs.rmSync(staged, { force: true }); } catch {}
    }
    return destination;
  } finally {
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

function markNetworkError(error) {
  if (['ENOTFOUND', 'ECONNRESET', 'ETIMEDOUT', 'EAI_AGAIN', 'ECONNREFUSED'].includes(error.code) || (error.statusCode >= 500 && error.statusCode <= 599) || /^(Download idle timeout|Total download timed out)$/.test(error.message)) error.network = true;
  if (error.statusCode === 404) error.releaseNotReady = true;
}

// On Windows a bare `tar` can resolve to GNU tar (e.g. Git Bash), which reads `C:\path` as host:path and cannot
// extract zip. Use the bsdtar that ships with Windows 10 1803+ explicitly.
function tarCommand() {
  if (process.platform === 'win32') {
    if (!process.env.SystemRoot) throw new Error('SystemRoot is required to locate Windows bsdtar (System32\\tar.exe)');
    const system = path.join(process.env.SystemRoot, 'System32', 'tar.exe');
    if (!fs.existsSync(system)) throw new Error(`Windows bsdtar was not found at ${system}; install a supported Windows version`);
    return system;
  }
  return 'tar';
}

module.exports = { ALLOWED_HOSTS, MAX_DOWNLOAD_BYTES, MAX_EXTRACTED_BINARY_BYTES, MIN_BINARY_BYTES, RELEASE_BASE, downloadBinary, normalizeHost, parseChecksum, requestBuffer, tarCommand, verifySha256 };
