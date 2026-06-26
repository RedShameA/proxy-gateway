#!/bin/sh
set -e

if [ "$(stat -c '%U' /data 2>/dev/null)" != "app" ]; then
    chown -R app:app /data
fi

exec su-exec app "$@"
