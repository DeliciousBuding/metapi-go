#!/usr/bin/env python3
"""Deterministic OpenAI-compatible upstream for the strict relay E2E gate."""

import argparse
import json
import os
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

DEFAULT_MODEL = "gpt-4o-mini"
DEFAULT_MARKER = "metapi-e2e-marker"

CHAT_PATH = "/v1/chat/completions"
MESSAGES_PATH = "/v1/messages"
RESPONSES_PATH = "/v1/responses"


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default=os.environ.get("MOCK_OPENAI_HOST", "127.0.0.1"))
    parser.add_argument("--port", type=int, default=int(os.environ.get("MOCK_OPENAI_PORT", "4200")))
    parser.add_argument("--log", default=os.environ.get("MOCK_OPENAI_LOG", ""))
    return parser.parse_args()


class Handler(BaseHTTPRequestHandler):
    server_version = "metapi-e2e-mock/1"

    def log_message(self, _format, *_args):
        return

    def write_json(self, status, payload):
        encoded = json.dumps(payload, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def do_GET(self):
        if self.path == "/health":
            self.write_json(200, {"status": "ok"})
            return
        if self.path == "/v1/models":
            models = [item.strip() for item in os.environ.get("MOCK_OPENAI_MODELS", DEFAULT_MODEL).split(",") if item.strip()]
            self.write_json(200, {"object": "list", "data": [{"id": model, "object": "model"} for model in models]})
            return
        self.write_json(404, {"error": {"message": "not found", "type": "mock_not_found"}})

    def do_POST(self):
        if self.path not in (CHAT_PATH, MESSAGES_PATH, RESPONSES_PATH):
            self.write_json(404, {"error": {"message": "not found", "type": "mock_not_found"}})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            payload = json.loads(self.rfile.read(length) or b"{}")
        except (ValueError, json.JSONDecodeError):
            self.write_json(400, {"error": {"message": "invalid JSON", "type": "invalid_request_error"}})
            return

        model = payload.get("model")
        self.server.record({"path": self.path, "model": model})
        if os.environ.get("MOCK_OPENAI_COMPLETION", "ok") == "error":
            self.write_json(502, {"error": {"message": "forced structured error", "type": "mock_error"}})
            return

        marker = os.environ.get("MOCK_OPENAI_MARKER", DEFAULT_MARKER)
        if self.path == MESSAGES_PATH:
            if payload.get("stream"):
                # Honest refusal beats a fake stream: the relay gates assert
                # non-streaming JSON only, and this mock must not pretend
                # otherwise.
                self.write_json(400, {"error": {"message": "streaming is not supported by this mock", "type": "mock_unsupported_stream"}})
                return
            self.write_json(
                200,
                {
                    "id": "msg_metapi_e2e",
                    "type": "message",
                    "role": "assistant",
                    "model": model,
                    "content": [{"type": "text", "text": marker}],
                    "stop_reason": "end_turn",
                    "stop_sequence": None,
                    "usage": {"input_tokens": 1, "output_tokens": 1},
                },
            )
            return
        if self.path == RESPONSES_PATH:
            if payload.get("stream"):
                self.write_json(400, {"error": {"message": "streaming is not supported by this mock", "type": "mock_unsupported_stream"}})
                return
            self.write_json(
                200,
                {
                    "id": "resp_metapi_e2e",
                    "object": "response",
                    "created_at": int(time.time()),
                    "status": "completed",
                    "model": model,
                    "output": [
                        {
                            "type": "message",
                            "id": "msg_metapi_e2e_output",
                            "status": "completed",
                            "role": "assistant",
                            "content": [{"type": "output_text", "text": marker, "annotations": []}],
                        }
                    ],
                    "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
                },
            )
            return
        self.write_json(
            200,
            {
                "id": "chatcmpl-metapi-e2e",
                "object": "chat.completion",
                "created": int(time.time()),
                "model": model,
                "choices": [
                    {
                        "index": 0,
                        "message": {"role": "assistant", "content": marker},
                        "finish_reason": "stop",
                    }
                ],
                "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
            },
        )


class Server(ThreadingHTTPServer):
    def __init__(self, address, log_path):
        super().__init__(address, Handler)
        self.log_path = Path(log_path) if log_path else None
        self.log_lock = threading.Lock()

    def record(self, event):
        if self.log_path is None:
            return
        self.log_path.parent.mkdir(parents=True, exist_ok=True)
        with self.log_lock, self.log_path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(event, separators=(",", ":")) + "\n")


def main():
    args = parse_args()
    server = Server((args.host, args.port), args.log)
    print(f"mock OpenAI listening on {args.host}:{args.port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
