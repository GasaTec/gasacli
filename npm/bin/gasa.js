#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { getPlatform } = require('../lib/platform');
const { downloadBinary } = require('../lib/download');

async function main() {
  const { binary } = getPlatform();
  const executable = path.resolve(__dirname, '..', 'vendor', binary);
  let vendorValid = false;
  try {
    const stat = fs.lstatSync(executable);
    vendorValid = stat.isFile() && stat.size >= 1024;
  } catch {}
  if (!vendorValid) await downloadBinary();
  const child = spawn(executable, process.argv.slice(2), { stdio: 'inherit' });
  const removeSignalListeners = installSignalForwarding(child, process);
  child.on('error', (error) => { removeSignalListeners(); console.error(`gasacli: could not start binary: ${error.message}`); process.exitCode = 1; });
  child.on('exit', (code, signal) => {
    handleChildExit(process, removeSignalListeners, code, signal);
  });
}

function installSignalForwarding(child, processObject) {
  const listeners = new Map();
  for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP']) {
    const listener = () => { if (!child.killed) child.kill(signal); };
    listeners.set(signal, listener);
    processObject.on(signal, listener);
  }
  return () => {
    for (const [signal, listener] of listeners) processObject.removeListener(signal, listener);
  };
}

function handleChildExit(processObject, removeSignalListeners, code, signal) {
  removeSignalListeners();
  if (signal) processObject.kill(processObject.pid, signal);
  else processObject.exitCode = code === null ? 1 : code;
}

if (require.main === module) {
  main().catch((error) => {
    console.error(`gasacli: ${error.message}`);
    process.exitCode = 1;
  });
}

module.exports = { handleChildExit, installSignalForwarding };
