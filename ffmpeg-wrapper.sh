#!/bin/sh
# Intercepts and strictly removes the deprecated -vol argument that jonas747/dca structurally injects rigidly
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

# Actively proxy directly back into the core FFMPEG natively.
# We physically append -nostats inherently to perfectly suppress the FFMPEG 6+ telemetry
# that physically crashes the dca fmt.Sscanf legacy parser loop completely!
exec /usr/bin/ffmpeg -nostats "$@"
