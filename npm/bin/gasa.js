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
  launch(executable, process.argv.slice(2), process, false);
}

function launch(executable, args, processObject, recovered) {
  let child;
  try { child = spawn(executable, args, { stdio: 'inherit' }); }
  catch (error) { child = new EventEmitter(); process.nextTick(() => child.emit('error', error)); }
  const removeSignalListeners = installSignalForwarding(child, processObject);
  child.on('error', async (error) => {
    removeSignalListeners();
    const recoverable = ['EFTYPE', 'ENOEXEC', 'UNKNOWN'].includes(error.code) || /Exec format error/i.test(error.message);
    if (!recoverable) {
      const reason = error.code === 'EACCES' ? 'permission denied; vendor binary was preserved' : error.code === 'ENOENT' ? 'binary is missing; vendor directory was preserved' : 'binary could not start; vendor directory was preserved';
      console.error(`gasacli: ${reason}: ${error.message}`); processObject.exitCode = 1; return;
    }
    if (!recovered) {
      try {
        fs.rmSync(path.dirname(executable), { recursive: true, force: true });
        await downloadBinary();
        launch(executable, args, processObject, true);
        return;
      } catch (retryError) { console.error(`gasacli: binary could not start and recovery download failed: ${retryError.message}`); processObject.exitCode = 1; return; }
    }
    console.error(`gasacli: binary could not start after one verified recovery download: ${error.message}`);
    processObject.exitCode = 1;
  });
  child.on('exit', (code, signal) => {
    handleChildExit(processObject, removeSignalListeners, code, signal);
  });
}

const { EventEmitter } = require('node:events');

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

module.exports = { handleChildExit, installSignalForwarding, launch };
