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

Usage: httpbin_server.py [port] [--port-file PATH]

Port 0 asks the OS for an available loopback port. When --port-file is given,
the selected port is written atomically before requests are served.
"""

import json
import os
import sys
import tempfile
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
    args = sys.argv[1:]
    port = int(args.pop(0)) if args and not args[0].startswith("--") else 9190
    port_file = None
    if args:
        if len(args) != 2 or args[0] != "--port-file":
            raise SystemExit("usage: httpbin_server.py [port] [--port-file PATH]")
        port_file = args[1]

    server = ThreadingHTTPServer(("127.0.0.1", port), Handler)
    if port_file:
        parent = os.path.dirname(os.path.abspath(port_file))
        fd, temporary = tempfile.mkstemp(prefix="httpbin-port-", dir=parent)
        try:
            with os.fdopen(fd, "w", encoding="ascii") as stream:
                stream.write(str(server.server_address[1]))
                stream.write("\n")
            os.replace(temporary, port_file)
        except BaseException:
            try:
                os.unlink(temporary)
            except FileNotFoundError:
                pass
            raise
    server.serve_forever()


if __name__ == "__main__":
    main()
