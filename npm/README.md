# gasacli npm package

This package downloads the matching GasaCLI release binary from GitHub Releases during installation. If that download is unavailable, the `gasa` command retries on first run.

```sh
npm install --global gasacli
npx gasacli -- --version
```

Release assets come only from `https://github.com/GasaTec/gasacli/releases/download/` and are checked against the release `checksums.txt` before installation.

The published npm package version is set to match its release tag. The repository's `0.0.0` version is a publishing placeholder.
