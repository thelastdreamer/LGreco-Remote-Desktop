#!/bin/bash
set -e

export DISPLAY=${DISPLAY:-:99}
RESOLUTION=${RESOLUTION:-1280x720}
WIDTH=$(echo $RESOLUTION | cut -d'x' -f1)
HEIGHT=$(echo $RESOLUTION | cut -d'x' -f2)

mkdir -p /tmp/.X11-unix
Xvfb $DISPLAY -screen 0 ${WIDTH}x${HEIGHT}x24 -ac +extension RANDR &
sleep 1

if [ -n "$TARGET_HOST" ] && [ -n "$TARGET_USER" ]; then
    xfreerdp /v:${TARGET_HOST}:${TARGET_PORT:-3389} \
        /u:${TARGET_USER} /p:${TARGET_PASS:-} \
        /size:${WIDTH}x${HEIGHT} \
        /cert-ignore \
        +fonts +clipboard \
        /sound:sys:pulse &
    sleep 3
else
    xfce4-session &
    sleep 2
fi

x11vnc -display $DISPLAY -forever -nopw -quiet -listen 0.0.0.0 -xkb &
sleep 1

websockify --web /usr/share/novnc 8081 localhost:5900 &
sleep 1

python3 /gst-pipeline.py &

wait
