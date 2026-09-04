# Use the package version as the release identity

Yop CLI declares its release version in `package.json` before tagging, and requires the lockfile, Git tag, binary, archives, GitHub Release, and npm package to match it. Stable and Beta use separate ordered channels, and publication advances only after the same checksummed Release Candidate passes verification; Apple signing and notarization remain disabled until credentials are configured. This replaces the easier but unsafe model that rewrote package metadata after GoReleaser had already built and published artifacts.
