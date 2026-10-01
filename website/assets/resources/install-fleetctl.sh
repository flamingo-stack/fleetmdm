#!/bin/bash

set -e

FLEETCTL_INSTALL_DIR="${HOME}/.fleetctl/"


# Check for necessary commands
for cmd in curl tar grep sed shasum; do
    if ! command -v $cmd &> /dev/null; then
        echo "Error: $cmd is not installed." >&2
        exit 1
    fi
done

echo "Fetching the latest version of fleetctl..."


# Fetch the latest version number from NPM
latest_strippedVersion=$(curl -s "https://registry.npmjs.org/fleetctl/latest" | grep -o '"version": *"[^"]*"' | cut -d'"' -f4)
echo "Latest version available on NPM: $latest_strippedVersion"

# Validate that the version string looks like a semver value before using it
# in a URL or filesystem path.
if ! [[ "$latest_strippedVersion" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Unexpected version string received from NPM: '${latest_strippedVersion}'" >&2
    exit 1
fi

version_gt() {
  test "$(printf '%s\n' "$@" | sort -V | head -n 1)" != "$1";
}

# Determine operating system (Linux or MacOS)
OS="$(uname -s)"

# Determine architecture (amd64 or arm64)
if uname -m | grep -qE '^(arm|aarch64)';
then
  ARCH="arm64";
else
  ARCH="amd64";
fi

# Standardize OS name for file download
case "${OS}" in
    Linux*)     OS="linux_${ARCH}" OS_DISPLAY_NAME='Linux';;
    Darwin*)    OS='macos' OS_DISPLAY_NAME='macOS';;
    *)          echo "Unsupported operating system: ${OS}"; exit 1;;
esac

# Create the install directory if it does not exist.
mkdir -p "${FLEETCTL_INSTALL_DIR}"

# Construct download URL
# ex: https://github.com/fleetdm/fleet/releases/download/fleet-v4.43.3/fleetctl_v4.43.3_macos.zip
DOWNLOAD_URL="https://github.com/fleetdm/fleet/releases/download/fleet-v${latest_strippedVersion}/fleetctl_v${latest_strippedVersion}_${OS}.tar.gz"
CHECKSUMS_URL="https://github.com/fleetdm/fleet/releases/download/fleet-v${latest_strippedVersion}/fleetctl_v${latest_strippedVersion}.checksums.txt"

# Download the latest version of fleetctl, verify its checksum, then extract it.
echo "Downloading fleetctl ${latest_strippedVersion} for ${OS_DISPLAY_NAME}..."

TMP_DIR=$(mktemp -d)
trap 'rm -rf "${TMP_DIR}"' EXIT

ARCHIVE_NAME="fleetctl_v${latest_strippedVersion}_${OS}.tar.gz"
ARCHIVE_PATH="${TMP_DIR}/${ARCHIVE_NAME}"
CHECKSUMS_PATH="${TMP_DIR}/fleetctl_v${latest_strippedVersion}.checksums.txt"

curl -sSL -o "${ARCHIVE_PATH}" "$DOWNLOAD_URL"
curl -sSL -o "${CHECKSUMS_PATH}" "$CHECKSUMS_URL"

EXPECTED_SHA=$(grep "  ${ARCHIVE_NAME}\$" "${CHECKSUMS_PATH}" | awk '{print $1}')
if [[ -z "$EXPECTED_SHA" ]]; then
    echo "Error: Could not find checksum for ${ARCHIVE_NAME} in checksums file." >&2
    exit 1
fi

ACTUAL_SHA=$(shasum -a 256 "${ARCHIVE_PATH}" | awk '{print $1}')
if [[ "$EXPECTED_SHA" != "$ACTUAL_SHA" ]]; then
    echo "Error: Checksum verification failed for ${ARCHIVE_NAME}." >&2
    echo "Expected: ${EXPECTED_SHA}" >&2
    echo "Actual:   ${ACTUAL_SHA}" >&2
    exit 1
fi

tar -xz -f "${ARCHIVE_PATH}" -C "$FLEETCTL_INSTALL_DIR" --strip-components=1 fleetctl_v"${latest_strippedVersion}"_${OS}/
echo "fleetctl installed successfully in ${FLEETCTL_INSTALL_DIR}"

# Verify if the binary is executable
if [[ ! -x "${FLEETCTL_INSTALL_DIR}/fleetctl" ]]; then
    echo "Failed to install or upgrade fleetctl. Please check your permissions and try running this script again."
    exit 1
fi

