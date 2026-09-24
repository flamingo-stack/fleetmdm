#!/bin/bash
# CIS 3.5 - Ensure Access to Audit Records Is Controlled
# The query requires exact permissions:
#   - /etc/security/audit_control: mode 0400, owned by root:wheel
#   - Entries in /var/audit/ AND in the `dir:` configured inside
#     audit_control: mode 0440, owned by root:wheel
# The original script only covered /var/audit; hosts with a
# customized `dir:` setting would still fail the query.

# /etc/security/audit_control must be exactly 0400
/usr/bin/sudo /usr/sbin/chown root:wheel /etc/security/audit_control
/usr/bin/sudo /bin/chmod 0400 /etc/security/audit_control

# Collect audit directories: always /var/audit, plus the `dir:` line
# from /etc/security/audit_control if it's configured to something
# different.
AUDIT_DIRS=("/var/audit")
CONFIGURED_DIR="$(/usr/bin/sudo /usr/bin/awk -F: '/^dir:/ { print $2; exit }' /etc/security/audit_control | /usr/bin/tr -d '[:space:]')"
# Validate CONFIGURED_DIR is an absolute path under a safe root before
# ever using it in find/chmod/chown, to avoid a misconfigured or
# malformed `dir:` line (e.g. "/" or a relative path) causing these
# commands to operate far outside the intended audit directories.
case "$CONFIGURED_DIR" in
    /var/audit)
        ;;
    /var/audit/*|/private/var/audit/*)
        if [ -n "$CONFIGURED_DIR" ]; then
            AUDIT_DIRS+=("$CONFIGURED_DIR")
        fi
        ;;
    *)
        if [ -n "$CONFIGURED_DIR" ]; then
            echo "Refusing to use unsafe configured audit dir: '$CONFIGURED_DIR'" >&2
        fi
        ;;
esac

for dir in "${AUDIT_DIRS[@]}"; do
    if [ -d "$dir" ]; then
        /usr/bin/sudo /usr/sbin/chown -R root:wheel "$dir"
        # The query uses `path LIKE '/var/audit/%'` which also matches
        # subdirectories. Chmod every entry under the dir (file or
        # directory) to 0440 to keep the query satisfied.
        /usr/bin/sudo /usr/bin/find "$dir" -mindepth 1 -exec /bin/chmod 0440 {} \;
    fi
done

