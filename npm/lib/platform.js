'use strict';

function getPlatform(platform = process.platform, arch = process.arch) {
  const osNames = { win32: 'windows', linux: 'linux', darwin: 'darwin' };
  const archNames = { x64: 'amd64', arm64: 'arm64' };
  const os = osNames[platform];
  const architecture = archNames[arch];
  if (!os || !architecture) {
    throw new Error(`Unsupported platform: ${platform}/${arch}. Supported: Windows, Linux, and macOS on x64 or arm64.`);
  }
  return { os, arch: architecture, extension: os === 'windows' ? 'zip' : 'tar.gz', binary: os === 'windows' ? 'gasa.exe' : 'gasa' };
}

module.exports = { getPlatform };
