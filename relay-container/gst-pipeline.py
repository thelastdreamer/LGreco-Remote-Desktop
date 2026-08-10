#!/usr/bin/env python3
"""
GStreamer WebRTC pipeline for remote desktop streaming.
Captures X11 display and PulseAudio, streams via WebRTC to signaling server.
"""

import os
import sys
import json
import asyncio
import aiohttp
import gi
import logging

gi.require_version('Gst', '1.0')
gi.require_version('GstWebRTC', '1.0')
gi.require_version('GstSdp', '1.0')

from gi.repository import Gst, GstWebRTC, GstSdp, GLib

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("gst-pipeline")

Gst.init(None)

SIGNAL_URL = os.environ.get("SIGNAL_URL", "ws://api:8080/ws/signal?session=1&role=host")
STUN_SERVER = os.environ.get("STUN_SERVER", "stun:stun.l.google.com:19302")
RESOLUTION = os.environ.get("RESOLUTION", "1280x720")
WIDTH, HEIGHT = RESOLUTION.split("x")
DISPLAY = os.environ.get("DISPLAY", ":99")
FRAMERATE = os.environ.get("FRAMERATE", "30")
BITRATE = os.environ.get("BITRATE", "2000")


class WebRTCClient:
    def __init__(self):
        self.conn = None
        self.pipe = None
        self.webrtcbin = None
        self.loop = asyncio.get_event_loop()

    def build_pipeline(self):
        pipe_desc = (
            f"ximagesrc display-name={DISPLAY} use-damage=0 ! "
            f"videoconvert ! videoscale ! "
            f"video/x-raw,width={WIDTH},height={HEIGHT},framerate={FRAMERATE}/1 ! "
            f"x264enc tune=zerolatency bitrate={BITRATE} speed-preset=ultrafast ! "
            f"video/x-h264,profile=baseline ! h264parse ! "
            f"rtph264pay pt=96 ! "
            f"webrtcbin name=webrtcbin stun-server={STUN_SERVER}"
        )

        logger.info(f"Pipeline: {pipe_desc[:200]}...")
        self.pipe = Gst.parse_launch(pipe_desc)
        self.webrtcbin = self.pipe.get_by_name("webrtcbin")

        self.webrtcbin.connect("on-negotiation-needed", self._on_negotiation_needed)
        self.webrtcbin.connect("on-ice-candidate", self._on_ice_candidate)
        self.webrtcbin.connect("pad-added", self._on_pad_added)

    def _on_negotiation_needed(self, element):
        promise = Gst.Promise.new_with_change_func(self._on_offer_created, None, None)
        self.webrtcbin.emit("create-offer", None, promise)

    def _on_offer_created(self, promise, _):
        reply = promise.get_reply()
        offer = reply.get_value("offer")
        promise = Gst.Promise.new_with_change_func(self._on_local_description_set, None, None)
        self.webrtcbin.emit("set-local-description", offer, promise)

        sdp_text = offer.sdp.as_text()
        asyncio.run_coroutine_threadsafe(self._send_sdp("offer", sdp_text), self.loop)

    def _on_local_description_set(self, promise, _):
        logger.info("Local description set")

    def _on_ice_candidate(self, element, sdp_mline_index, candidate):
        if candidate:
            asyncio.run_coroutine_threadsafe(
                self._send_ice(candidate, sdp_mline_index), self.loop
            )

    def _on_pad_added(self, element, pad):
        logger.info(f"Pad added: {pad.get_name()}")

    async def _send_sdp(self, sdp_type, sdp):
        msg = json.dumps({"type": sdp_type, "sdp": sdp})
        await self._ws_send(msg)

    async def _send_ice(self, candidate, sdp_mline_index):
        candidate_str = f"candidate:{candidate.candidate}"
        msg = json.dumps({
            "type": "ice-candidate",
            "candidate": {
                "candidate": candidate_str,
                "sdpMLineIndex": sdp_mline_index,
            }
        })
        await self._ws_send(msg)

    async def _ws_send(self, msg):
        if self.conn:
            try:
                await self.conn.send_str(msg)
            except Exception as e:
                logger.error(f"WS send error: {e}")

    async def _ws_recv(self):
        try:
            async for msg in self.conn:
                if msg.type == aiohttp.WSMsgType.TEXT:
                    data = json.loads(msg.data)
                    await self._handle_message(data)
                elif msg.type == aiohttp.WSMsgType.ERROR:
                    logger.error(f"WS error: {self.conn.exception()}")
                    break
        except Exception as e:
            logger.error(f"WS recv error: {e}")

    async def _handle_message(self, data):
        msg_type = data.get("type", "")
        if msg_type == "answer":
            sdp_text = data.get("sdp", "")
            if sdp_text:
                _, sdp_msg = GstSdp.SDPMessage.new_from_text(sdp_text)
                answer = GstWebRTC.WebRTCSessionDescription.new(
                    GstWebRTC.WebRTCSDPType.ANSWER, sdp_msg
                )
                promise = Gst.Promise.new_with_change_func(
                    lambda p, _: logger.info("Remote description set"), None, None
                )
                self.webrtcbin.emit("set-remote-description", answer, promise)
        elif msg_type == "ice-candidate":
            cand = data.get("candidate", {})
            candidate_str = cand.get("candidate", "")
            sdp_mline_index = cand.get("sdpMLineIndex", 0)
            if candidate_str and candidate_str.startswith("candidate:"):
                candidate_str = candidate_str[len("candidate:"):]
            if candidate_str:
                self.webrtcbin.emit("add-ice-candidate", sdp_mline_index, candidate_str)

    async def connect(self):
        reconnect_delay = 1
        while True:
            try:
                session = aiohttp.ClientSession()
                self.conn = await session.ws_connect(SIGNAL_URL)
                logger.info(f"Connected to signaling server: {SIGNAL_URL}")

                self.build_pipeline()
                self.pipe.set_state(Gst.State.PLAYING)

                await self._ws_recv()
            except Exception as e:
                logger.error(f"Connection error: {e}, reconnecting in {reconnect_delay}s...")
                await asyncio.sleep(reconnect_delay)
                reconnect_delay = min(reconnect_delay * 2, 30)
            finally:
                if self.pipe:
                    self.pipe.set_state(Gst.State.NULL)


if __name__ == "__main__":
    client = WebRTCClient()
    asyncio.run(client.connect())
