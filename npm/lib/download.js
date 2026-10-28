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

function parseChecksum(text, archiveName) {
  for (const line of text.split(/\r?\n/)) {
    const match = /^([a-f\d]{64})\s+\*?(.+?)\s*$/i.exec(line);
    if (match && match[2] === archiveName) return match[1].toLowerCase();
  }
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
  const protocolAllowed = parsed.protocol === 'https:' || (options.allowHttp && parsed.protocol === 'http:');
  const portAllowed = !parsed.port || parsed.port === '443' || (options.allowHttp && parsed.protocol === 'http:');
  if (!protocolAllowed || !portAllowed || !allowedHosts.has(parsed.hostname)) {
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
      fn(value);
    };
    const req = request.get(parsed, { headers: { 'User-Agent': 'gasacli-npm-installer' } }, (res) => {
      if ([301, 302, 303, 307, 308].includes(res.statusCode)) {
        res.resume();
        if (redirects >= 5) return finish(reject, new Error('Too many redirects while downloading release asset'));
        const location = res.headers.location;
        if (!location) return finish(reject, new Error('Redirect response did not include a location'));
        const next = new URL(location, parsed);
        const nextProtocolAllowed = next.protocol === 'https:' || (options.allowHttp && next.protocol === 'http:');
        const nextPortAllowed = !next.port || next.port === '443' || (options.allowHttp && next.protocol === 'http:');
        if (!allowedHosts.has(next.hostname) || !nextProtocolAllowed || !nextPortAllowed) {
          return finish(reject, new Error(`Download redirect host is not allowed: ${next.hostname}`));
        }
        return finish(resolve, requestBuffer(next.href, options, redirects + 1, startedAt));
      }
      if (res.statusCode !== 200) {
        res.resume();
        return finish(reject, new Error(`Download failed with HTTP ${res.statusCode}`));
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
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?$/.test(version)) throw new Error(`Invalid package release version: ${version}`);
  const target = options.vendorDir || path.resolve(__dirname, '..', 'vendor');
  const { os: platform, arch, extension, binary } = getPlatform(options.platform, options.arch);
  const archiveName = `gasa_${version}_${platform}_${arch}.${extension}`;
  const releaseBase = options.releaseBase || RELEASE_BASE;
  const prefix = `${releaseBase}/v${version}`;
  const checksumUrl = `${prefix}/checksums.txt`;
  const archiveUrl = `${prefix}/${archiveName}`;
  const budget = { bytes: 0 };
  const checksumsBuffer = await getResource(checksumUrl, options, budget);
  const expectedHash = parseChecksum(checksumsBuffer.toString('utf8'), archiveName);
  const archive = await getResource(archiveUrl, options, budget);
  verifySha256(archive, expectedHash);

  const tempRoot = fs.mkdtempSync(path.join(options.tempDir || os.tmpdir(), 'gasacli-'));
  try {
    const archivePath = path.join(tempRoot, archiveName);
    const extractedPath = path.join(tempRoot, 'extract');
    fs.writeFileSync(archivePath, archive, { mode: 0o600 });
    fs.mkdirSync(extractedPath);
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

module.exports = { ALLOWED_HOSTS, MAX_DOWNLOAD_BYTES, MAX_EXTRACTED_BINARY_BYTES, MIN_BINARY_BYTES, RELEASE_BASE, downloadBinary, parseChecksum, requestBuffer, tarCommand, verifySha256 };
