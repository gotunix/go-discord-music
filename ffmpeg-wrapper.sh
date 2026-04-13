#!/bin/sh
# Intercepts and strictly removes the deprecated -vol argument that jonas747/dca structurally injects rigidly, causing EOF crashes in modern Alpine versions safely natively!

for arg do
    shift
    if [ "$skip" = "1" ]; then
        unset skip
        continue
    fi
    if [ "$arg" = "-vol" ]; then
        skip="1"
        continue
    fi
    
    # Push back exactly securely preserving quotes and structural spacing natively
    set -- "$@" "$arg"
done

# Actively proxy directly back into the core FFMPEG mapped perfectly natively
exec /usr/bin/ffmpeg "$@"
