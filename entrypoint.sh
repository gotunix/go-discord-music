#!/bin/sh

PUID=${PUID:-1000}
PGID=${PGID:-1000}

if ! getent group abc >/dev/null; then
    addgroup -g "${PGID}" abc
fi

if ! getent passwd abc >/dev/null; then
    adduser -u "${PUID}" -G abc -h /app -D -s /bin/sh abc
fi

chown -R abc:abc /app
chown -R abc:abc /opt/venv 2>/dev/null || true

exec su-exec abc "$@"
