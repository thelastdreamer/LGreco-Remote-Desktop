FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV RESOLUTION=1280x720
ENV DISPLAY=:99
ENV TARGET_HOST=
ENV TARGET_PORT=3389
ENV TARGET_USER=
ENV TARGET_PASS=

RUN apt-get update && apt-get install -y --no-install-recommends \
    xvfb x11vnc xdotool \
    freerdp2-x11 \
    supervisor \
    novnc websockify \
    pulseaudio pulseaudio-utils \
    gstreamer1.0-tools gstreamer1.0-plugins-base gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly \
    gstreamer1.0-libav gstreamer1.0-nice \
    python3 python3-pip python3-websockets \
    x11-utils dbus-x11 \
    curl ca-certificates \
    libx264-163 \
    && rm -rf /var/lib/apt/lists/*

RUN pip3 install aiohttp

COPY entrypoint.sh /entrypoint.sh
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY gst-pipeline.py /gst-pipeline.py

RUN chmod +x /entrypoint.sh /gst-pipeline.py

EXPOSE 5900 8081

ENTRYPOINT ["/entrypoint.sh"]
