"""Loopback-only fixture for the output and plugin examples (Python 3)."""

import argparse
import json
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    gets = 0
    submissions = []

    def reply(self, status, body):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path == "/get":
            Handler.gets += 1
            self.reply(200, {"hello": "curlew"})
        elif self.path == "/observations":
            self.reply(200, {"gets": Handler.gets, "submissions": Handler.submissions})
        else:
            self.reply(404, {"error": "unknown path"})

    def do_POST(self):
        if self.path != "/api/v2/series":
            self.reply(404, {"error": "unknown path"})
            return
        try:
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
        except (ValueError, UnicodeDecodeError):
            self.reply(400, {"error": "invalid JSON"})
            return
        Handler.submissions.append(body)
        self.reply(202, {"accepted": True})

    def log_message(self, *_args):
        pass


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=18081)
    args = parser.parse_args()
    with HTTPServer(("127.0.0.1", args.port), Handler) as server:
        print(f"http://127.0.0.1:{server.server_port}", flush=True)
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            pass
