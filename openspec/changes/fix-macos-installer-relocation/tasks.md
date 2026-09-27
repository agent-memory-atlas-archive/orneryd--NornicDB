## macOS installer placement and ownership

- [x] Disable bundle relocation so the package targets `/Applications/NornicDB.app` instead of a matching bundle in build staging.
- [x] Remove the build script's privileged cleanup fallback; report inaccessible staging explicitly.
- [x] Write per-user install files as the user in both package variants, and report missing app or launch failures accurately.
- [x] Check shell syntax, package relocation metadata, and the missing-app failure path.
- [ ] Remove root-owned staging from the earlier relocatable install (one-time administrator action).
- [ ] Rebuild and install a fresh package, then verify the app exists in `/Applications` and no build-staging files are root-owned.