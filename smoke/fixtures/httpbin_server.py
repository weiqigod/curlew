#!/usr/bin/env python3
"""Local httpbin.org lookalike for smoke/run.sh.

The smoke gate must never execute requests against the public internet —
a slow httpbin.org patch alone has failed CI twice on network weather.
This server emulates the subset of httpbin's echo contract the smoke
checks assert on:

  - every path responds 200 with Content-Type: application/json
  - body is a JSON object with httpbin's field names:
      args     query parameters (single string values)
      headers  request headers as sent (Go canonicalises to Content-Type form)
      url      full request URL (scheme://host/path?query)
      method   request method
      data     raw request body decoded as UTF-8 ("" if binary), bodied methods only
      json     parsed body when Content-Type is JSON, else null, bodied methods only

Usage: httpbin_server.py [port]   (binds 127.0.0.1, default port 9190)
"""

import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qsl, urlparse


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):  # keep smoke output clean
        pass

    def _echo(self, body):
        parsed = urlparse(self.path)
        payload = {
            "args": dict(parse_qsl(parsed.query)),
            "headers": dict(self.headers.items()),
            "url": "http://%s%s" % (self.headers.get("Host", "127.0.0.1"), self.path),
            "method": self.command,
        }
        if body is not None:
            try:
                payload["data"] = body.decode("utf-8")
            except UnicodeDecodeError:
                payload["data"] = ""
            payload["json"] = None
            if "json" in (self.headers.get("Content-Type") or ""):
                try:
                    payload["json"] = json.loads(body)
                except ValueError:
                    pass
        out = json.dumps(payload).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)

    def _read_body(self):
        length = int(self.headers.get("Content-Length") or 0)
        return self.rfile.read(length)

    def do_GET(self):
        self._echo(None)

    def do_POST(self):
        self._echo(self._read_body())

    do_PUT = do_POST
    do_PATCH = do_POST
    do_DELETE = do_GET


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9190
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()


if __name__ == "__main__":
    main()
