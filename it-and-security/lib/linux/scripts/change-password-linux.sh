#!/bin/sh

# Disable automatic login for common display managers
disable_autologin() {
    # GDM (GNOME Display Manager)
    if [ -f /etc/gdm3/custom.conf ]; then
        sed -i '/^AutomaticLoginEnable/s/^/#/' /etc/gdm3/custom.conf
        sed -i '/^AutomaticLogin/s/^/#/' /etc/gdm3/custom.conf
    fi

    # LightDM
    if [ -f /etc/lightdm/lightdm.conf ]; then
        sed -i '/^autologin-user=/s/^/#/' /etc/lightdm/lightdm.conf
    fi

    # Add similar cases for other display managers if needed
}

# Disable automatic login
disable_autologin

# Directory to securely store generated passwords, readable only by root
password_dir="/root/.change-password-linux"
mkdir -p "$password_dir"
chmod 700 "$password_dir"

# Loop through all users in /etc/passwd
awk -F':' '{ if ($3 >= 1000 && $3 < 60000) print $1 }' /etc/passwd | while read user
do
    if [ "$user" != "root" ]; then
        echo "Logging out $user"
        pkill -KILL -u "$user" # Kill user processes. This will log out logged-in users.
        password=$(openssl rand -base64 9) 
        echo "$user:$password" | chpasswd
        password_file="$password_dir/$user.password"
        printf '%s\n' "$password" > "$password_file"
        chmod 600 "$password_file"
        echo "New password for $user written to $password_file"
    fi
done

echo "All non-root users have been logged out and their passwords changed."

