# Publishing updateable Docker releases

The application does not update from arbitrary tags, shell scripts or unsigned checksums. A release contains `update.json` and `update.json.sig`; its Ed25519 signature covers the stable version, minimum supported source version, expected SQLite schema, immutable GHCR image digest and plain-text notes. The signature is verified against the public key embedded in the installed image (or explicitly configured by its operator).

## One-time publisher setup

1. Keep a private key outside this repository and all site data/build contexts. Generate it offline with:

   ```sh
   go run ./cmd/release-manifest keygen ../gopress-release.key
   ```

   The command prints only the public key. The new private-key file has mode 0600 and cannot overwrite an existing file. Store an independent protected copy. Losing it prevents signing updates trusted by current installations.
2. Put the base64 private-key file contents in the repository Actions secret `GOPRESS_RELEASE_PRIVATE_KEY`. Do not print it into logs or commit it. Release jobs derive the public key and embed only that public key in the image.
3. Enable Actions package publication and ensure both the repository releases and `ghcr.io/OWNER/REPOSITORY` package are public if updates must be accessible without registry credentials. GitHub may initially create a private package; set its visibility explicitly before advertising the release.
4. Protect release tags, the release workflow and production publishing permissions. The signing key is a release authority; changing an image digest without authorization must not produce a trusted release.

## Publish

Create and push a reviewed stable `vMAJOR.MINOR.PATCH` tag. Run **Publish signed Docker release** against that tag, supplying the minimum compatible source version. The expected SQLite schema is derived from the tagged source (`store.SchemaVersion`, currently 17), and the signing tool rejects a different schema. The workflow validates the tag, runs Go/JS/browser checks, builds Linux amd64/arm64 images, checks Docker lifecycle behavior, verifies the pushed image's reported version, signs its digest and creates a GitHub release. Nothing is published by an ordinary branch push.

The image tag is informational; consumers install the signed `@sha256:` digest. Do not republish a version with different contents or modify its manifest assets. Use a new version instead. `RELEASE_VERSION`, `UPDATE_PUBLIC_KEY`, `IMAGE_REPOSITORY` and `UPDATE_FEED` are public Docker build arguments; the private signing key is never a build argument and never enters the image.

The first installation of this mechanism is a bootstrap: deploy a reviewed image with the correct public key and Dokploy environment settings once. Subsequent compatible releases can be installed through **Settings → Updates**. Native binary replacement and Dokploy Compose upgrades are outside this first implementation.

## Compatibility and failure testing

Before publishing, test an update from the declared minimum version using independent data. Include content, translations, forms, uploads, editable templates and queued mail. Test missing signatures, wrong keys, changed manifests, failed backup creation, incorrect Dokploy targets, lost deployment responses, process interruption and failed startup/migration. Test restoration with the recorded backup and compatible old image. A platform image rollback cannot reverse database migrations.

Mocked integration tests validate requests and state transitions. They do not establish that a particular production Dokploy version, registry account, node placement or mounted volume is configured correctly. Run the real container update/recovery trial before claiming production verification. No production update or release is launched merely by developing this feature.

## Key rotation

Automatic key rotation is not implemented in this first channel. Retain the same trust anchor for ordinary button updates. Changing the embedded public key also affects verification of an in-progress installation, so rotation requires an explicit operator bootstrap or a separately implemented and tested dual-key transition. Operators overriding `GOPRESS_UPDATE_PUBLIC_KEY` must update that override explicitly. An unexpected key change fails verification; never accept a replacement trust key from the release feed itself.

### Acceptance record for the schema 17 development build

Local automated checks and isolated browser acceptance are separate from real deployment acceptance. Before publishing this build, use a disposable Dokploy application with a separate persistent `/data` volume and matching trust key. Never point the trial at a working site's data.

1. Deploy the minimum supported source image; create a translated page, image, scheduled draft, editable template, form and enabled 2FA account. Keep recovery codes separately. Save a verified off-host backup.
2. Publish a **test-channel** signed manifest for the candidate immutable digest. Confirm that Check does not deploy, and that Update creates a verified backup before mutating Dokploy. Check the recorded previous digest and volume/node placement.
3. After replacement, verify the actual binary version, SQLite schema 17, completion state, restored pre-update maintenance state and all fixture content. Check public language links, file editing, login with a fresh second-factor code and session revocation.
4. Repeat with an invalid signature, unavailable backup destination, denied Dokploy API, unavailable image and interrupted deployment response. No failed preflight may deploy; an ambiguous remote result must retain maintenance/write blocking until an operator resolves it.
5. Test a failed startup/migration. Stop the application, restore the verified pre-upgrade data to a separate volume with the recorded old digest, verify it offline, then launch that recovery installation. Image rollback alone is insufficient after migrations. Normal backups omit authenticator seeds; use recovery codes or a separately protected security-directory backup.

Docker/Dokploy, external SMTP delivery, production DNS/TLS and off-host copy success must be recorded from the actual test environment. They were not established by mocked API tests or by writing this checklist. No release tag, image publication or production deployment is part of local development acceptance.
