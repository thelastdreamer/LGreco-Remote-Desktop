#!/bin/bash
set -e

export DISPLAY=${DISPLAY:-:99}
export PULSE_SERVER=unix:/run/pulse/native

RESOLUTION=${RESOLUTION:-1280x720}
WIDTH=$(echo $RESOLUTION | cut -d'x' -f1)
HEIGHT=$(echo $RESOLUTION | cut -d'x' -f2)

mkdir -p /run/dbus
if [ -f /var/lib/dbus/machine-id ]; then
    dbus-daemon --system --fork
fi

mkdir -p /tmp/.X11-unix
Xvfb $DISPLAY -screen 0 ${WIDTH}x${HEIGHT}x24 -ac +extension RANDR &
sleep 1

export DISPLAY=$DISPLAY
xfce4-session &
sleep 2

x11vnc -display $DISPLAY -forever -nopw -quiet -listen 0.0.0.0 -xkb &
sleep 1

mkdir -p /run/pulse
pulseaudio --start --exit-idle-time=-1 --disallow-exit --system=false &
sleep 1

websockify --web /usr/share/novnc 8081 localhost:5900 &
sleep 1

python3 /gst-pipeline.py &

wait
