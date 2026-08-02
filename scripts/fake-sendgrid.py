#!/usr/bin/env python3
"""Fake SendGrid v3 templates API for CI.

Implements the three endpoints SendGridTemplateUploader calls:
  GET  /v3/templates?generations=dynamic  — list templates (always returns empty)
  POST /v3/templates                      — create a template, returns a deterministic fake ID
  POST /v3/templates/{id}/versions        — create a version, always succeeds

Usage:
  python3 scripts/fake-sendgrid.py [port]   # default port: 8080
"""
import http.server
import itertools
import json
import sys


_id_seq = itertools.count(1)


class FakeSendGridHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path.startswith("/v3/templates"):
            self._json(200, {"templates": []})
        else:
            self._json(404, {"error": "not found"})

    def do_POST(self) -> None:
        # Consume request body so the connection is kept clean.
        content_length = int(self.headers.get("Content-Length", 0))
        if content_length > 0:
            self.rfile.read(content_length)

        if self.path == "/v3/templates":
            template_id = f"d-fake-{next(_id_seq):03d}"
            self._json(201, {"id": template_id})
        elif "/versions" in self.path:
            self._json(201, {"id": "v-1", "active": 1})
        else:
            self._json(404, {"error": "not found"})

    def _json(self, code: int, body: dict) -> None:
        data = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt: str, *args: object) -> None:
        pass  # suppress per-request logging for clean CI output


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8080
    server = http.server.HTTPServer(("127.0.0.1", port), FakeSendGridHandler)
    print(f"fake-sendgrid: listening on http://127.0.0.1:{port}", flush=True)
    server.serve_forever()
