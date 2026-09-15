#!/usr/bin/env python3
"""Smoke-test the actual executable's embedded UI, without opening a browser."""
import argparse
import json
from pathlib import Path
import queue
import re
import signal
import subprocess
import tempfile
import threading
import urllib.parse
import urllib.request


def check(binary):
    with tempfile.TemporaryDirectory(prefix="curlew-ui-artifact-") as project:
        subprocess.run([binary, "init"], cwd=project, check=True, capture_output=True)
        with tempfile.TemporaryFile(mode="w+") as errors:
            process = subprocess.Popen(
                [binary, "ui", "--no-open", "--port", "0"], cwd=project,
                stdout=subprocess.PIPE, stderr=errors, text=True,
            )
            try:
                lines = queue.Queue()
                threading.Thread(target=lambda: lines.put(process.stdout.readline()), daemon=True).start()
                line = lines.get(timeout=20).strip()
                if "listening on " not in line:
                    errors.seek(0)
                    raise RuntimeError("UI did not start: " + errors.read())
                url = line.split("listening on ", 1)[1]
                with urllib.request.urlopen(url, timeout=10) as response:
                    html = response.read().decode()
                resources = re.findall(r'(?:src|href)="([^\"]+\.(?:js|css))"', html)
                if not any(p.endswith(".js") for p in resources) or not any(p.endswith(".css") for p in resources):
                    raise RuntimeError("UI bundle is absent; build frontend before the executable")
                for resource in resources:
                    with urllib.request.urlopen(urllib.parse.urljoin(url, resource), timeout=10) as response:
                        content = response.read()
                        mime = response.headers.get("Content-Type", "")
                    expected = "javascript" if resource.endswith(".js") else "text/css"
                    if expected not in mime or not content or content.lstrip().lower().startswith(b"<!doctype html"):
                        raise RuntimeError("missing/wrong embedded asset: " + resource)
                parts = urllib.parse.urlsplit(url)
                token = urllib.parse.parse_qs(parts.query)["token"][0]
                tree_url = urllib.parse.urlunsplit((parts.scheme, parts.netloc, "/api/v1/tree", "", ""))
                request = urllib.request.Request(tree_url, headers={"Authorization": "Bearer " + token})
                with urllib.request.urlopen(request, timeout=10) as response:
                    tree = json.load(response)
                if not any(c["path"] == "collections/sample.yaml" and c["valid"] for c in tree["collections"]):
                    raise RuntimeError("UI cannot discover the freshly initialized collection")
                print("PASS: embedded JavaScript/CSS and authenticated collection discovery")
            finally:
                if process.poll() is None:
                    process.send_signal(signal.SIGINT)
                try:
                    process.communicate(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.communicate()
                    raise RuntimeError("UI did not stop after SIGINT")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", help="path to the executable to test")
    args = parser.parse_args()
    check(str(Path(args.binary).resolve()))
