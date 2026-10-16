#!/usr/bin/env bash
set -eu

TMP_PLIST=$(mktemp -t system.preferences)
trap 'rm -f "$TMP_PLIST"' EXIT
sudo security authorizationdb read system.preferences > "$TMP_PLIST"
defaults write "$TMP_PLIST" shared -bool false
sudo security authorizationdb write system.preferences < "$TMP_PLIST"

