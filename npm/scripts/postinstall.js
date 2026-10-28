'use strict';

const { downloadBinary } = require('../lib/download');

downloadBinary().catch((error) => {
  if (/SHA-256 mismatch|checksum|checksums\.txt/i.test(error.message)) {
    console.error(`gasacli: integrity check failed, binary NOT installed: ${error.message}`);
    process.exitCode = 1;
    return;
  }
  console.warn(`gasacli: binary download skipped because of a network or release availability error: ${error.message}`);
  console.warn('gasacli will retry the download the next time it is run.');
});
