# WebRTC Ion - Video Broadcasting System

A WebRTC-based video streaming system that uses Ion SFU (Selective Forwarding Unit) for efficient multi-party video communication. This project consists of a Go-based video broadcaster that captures camera input and streams it through an SFU server, with a web frontend to receive and display the video streams.

## Architecture

The project consists of three main components:

1. **Ion SFU Server** (`ion-sfu/`) - A WebRTC Selective Forwarding Unit that routes video/audio streams between peers
2. **Media Device Broadcaster** (`mediadevice-broadcast/`) - A Go application that captures video from a camera and broadcasts it to the SFU
3. **Web Frontend** (`public/` + `server.js`) - A simple Express.js server serving a web client that receives and displays video streams

## Features

- Real-time video streaming using WebRTC
- Camera capture using Pion mediadevices
- VP8 video codec encoding
- WebSocket-based signaling (JSON-RPC)
- Multi-client support through SFU architecture
- Simple web-based viewer interface

The broadcaster will:
- Connect to the SFU via WebSocket
- Capture video from your camera (640x480, YUY2 format)
- Encode video using VP8 codec (500 kbps bitrate)
- Stream the video to the SFU room "test room"

You should see the video stream appear in the web browser.

### Broadcaster Configuration

The broadcaster accepts command-line flags:
- `-a` - SFU address (default: `localhost:7000`)
