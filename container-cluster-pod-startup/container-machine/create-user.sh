#!/bin/sh
# First-boot user setup for the sample-app container machine; container machine's init runs
# /etc/machine/create-user.sh instead of its built-in script when this file exists.
# It creates the user exactly as the built-in script does (name, UID, GID, home, shell,
# passwordless sudo), but it never copies /etc/skel into an existing home and never
# changes owners below it: the home is the Mac's $HOME, mounted at the same path.
# Expects CONTAINER_USER, CONTAINER_UID, CONTAINER_GID, CONTAINER_HOME and CONTAINER_SHELL.
set -e

if ! getent group "${CONTAINER_GID}" >/dev/null 2>&1; then
    echo "${CONTAINER_USER}:x:${CONTAINER_GID}:" >> /etc/group
fi

if ! getent passwd "${CONTAINER_UID}" >/dev/null 2>&1; then
    echo "${CONTAINER_USER}:x:${CONTAINER_UID}:${CONTAINER_GID}::${CONTAINER_HOME}:${CONTAINER_SHELL}" >> /etc/passwd
    echo "${CONTAINER_USER}:!:19000:0:99999:7:::" >> /etc/shadow
fi

if [ ! -d "${CONTAINER_HOME}" ]; then
    mkdir -p "${CONTAINER_HOME}"
    chown "${CONTAINER_UID}:${CONTAINER_GID}" "${CONTAINER_HOME}"
fi

mkdir -p /etc/sudoers.d
sudoers_file=$(echo "${CONTAINER_USER}" | tr '.' '_')
echo "${CONTAINER_USER} ALL=(ALL) NOPASSWD:ALL" > "/etc/sudoers.d/${sudoers_file}"
chmod 440 "/etc/sudoers.d/${sudoers_file}"
