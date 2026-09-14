#!/bin/bash
# CIS 5.11 - Ensure Logging Is Enabled for Sudo
# Adds Defaults log_allowed to a sudoers.d file.
TMPFILE=$(/usr/bin/mktemp)
echo 'Defaults log_allowed' > "$TMPFILE"
if ! /usr/bin/sudo /usr/sbin/visudo -c -f "$TMPFILE" > /dev/null; then
    echo "Error: sudoers syntax validation failed for CIS_5_11_sudoconfiguration" >&2
    /bin/rm -f "$TMPFILE"
    exit 1
fi
/usr/bin/sudo /bin/cp "$TMPFILE" /etc/sudoers.d/CIS_5_11_sudoconfiguration
/bin/rm -f "$TMPFILE"
/usr/bin/sudo /bin/chmod 0440 /etc/sudoers.d/CIS_5_11_sudoconfiguration
if ! /usr/bin/sudo /usr/sbin/visudo -c -f /etc/sudoers.d/CIS_5_11_sudoconfiguration > /dev/null; then
    echo "Error: installed sudoers file failed validation, removing" >&2
    /usr/bin/sudo /bin/rm -f /etc/sudoers.d/CIS_5_11_sudoconfiguration
    exit 1
fi

