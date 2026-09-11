#!/bin/bash

# Get the current console user (the actual logged-in user), excluding root and loginwindow
CURRENT_USER=$(/usr/bin/stat -f "%Su" /dev/console)

if [[ -z "$CURRENT_USER" || "$CURRENT_USER" == "root" ]]; then
    echo "Unable to determine a valid non-root console user. Aborting."
    exit 1
fi

/usr/bin/sudo -u "$CURRENT_USER" /usr/bin/defaults write "/Users/$CURRENT_USER/Library/Preferences/.GlobalPreferences.plist" AppleShowAllExtensions -bool true

