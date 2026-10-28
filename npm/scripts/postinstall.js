'use strict';

const { downloadBinary } = require('../lib/download');

downloadBinary().catch((error) => {
  if (error.network || error.releaseNotReady) {
    console.warn(`gasacli: binary download skipped because of a network error: ${error.message}`);
    console.warn('gasacli will retry the download the next time it is run.');
  } else {
    console.error(`gasacli: integrity check failed, binary NOT installed: ${error.message}`);
    process.exitCode = 1;
  }
});
