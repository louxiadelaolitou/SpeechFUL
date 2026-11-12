#!/usr/bin/env python3
"""Bounded synthetic transport probe; never connect it to business services.

serve --info <private-new-file> starts an expiring loopback-only endpoint.
probe --info <same-file> [--origin https://assigned-host:port] measures it.
The info file contains a temporary capability and must stay out of Git.
"""

import argparse
import hashlib
import hmac
import json
import os
from pathlib import Path
import re
import secrets
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import (
    HTTPRedirectHandler, ProxyHandler, Request, build_opener,
)


MAX_BYTES = 6 * 1024 * 1024
MARKER = {"service": "speechful-synthetic-network-probe", "version": 1}


class BoundedServer(ThreadingHTTPServer):
    daemon_threads = True
    block_on_close = False

    def __init__(self, capability):
        self.capability = capability
        self.payload = secrets.token_bytes(MAX_BYTES)
        self.slots = threading.BoundedSemaphore(8)
        super().__init__(("127.0.0.1", 0), ProbeHandler)

    def process_request(self, request, client_address):
        if not self.slots.acquire(blocking=False):
            self.shutdown_request(request)
            return
        try:
            super().process_request(request, client_address)
        except Exception:
            self.slots.release()
            raise

    def process_request_thread(self, request, client_address):
        try:
            super().process_request_thread(request, client_address)
        finally:
            self.slots.release()


class ProbeHandler(BaseHTTPRequestHandler):
    server_version = "SyntheticProbe/1"
    sys_version = ""

    def setup(self):
        super().setup()
        self.connection.settimeout(10)

    def log_message(self, *args):
        pass  # Request paths contain the short-lived capability.

    def handle(self):
        try:
            super().handle()
        except (ConnectionError, TimeoutError):
            pass

    def reply(self, status, body=b"", mime="application/json", digest=None):
        self.send_response(status)
        self.send_header("Content-Type", mime)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("Connection", "close")
        if digest is not None:
            self.send_header("X-Content-SHA256", digest)
        self.end_headers()
        self.wfile.write(body)

    def route(self):
        parts = self.path.split("/")
        if len(parts) != 3 or not hmac.compare_digest(
            parts[1], self.server.capability
        ):
            return None
        return parts[2]

    def do_GET(self):
        route = self.route()
        if route == "health":
            self.reply(200, json.dumps(MARKER).encode())
        elif route and re.fullmatch(r"download-[0-9]{1,8}", route):
            size = int(route.split("-")[1])
            if not 1 <= size <= MAX_BYTES:
                self.reply(413)
                return
            body = self.server.payload[:size]
            self.reply(200, body, "application/octet-stream",
                       hashlib.sha256(body).hexdigest())
        else:
            self.reply(404)

    def do_PUT(self):
        if self.route() != "upload":
            self.reply(404)
            return
        lengths = self.headers.get_all("Content-Length", [])
        if self.headers.get("Transfer-Encoding") or len(lengths) != 1:
            self.reply(400)
            return
        if not re.fullmatch(r"[0-9]{1,8}", lengths[0]):
            self.reply(400)
            return
        size = int(lengths[0])
        if not 1 <= size <= MAX_BYTES:
            self.reply(413)
            return
        remaining, digest = size, hashlib.sha256()
        while remaining:
            chunk = self.rfile.read(min(65536, remaining))
            if not chunk:
                self.reply(400)
                return
            digest.update(chunk)
            remaining -= len(chunk)
        self.reply(200, json.dumps({
            "bytes": size, "sha256": digest.hexdigest(),
        }).encode())


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args):
        return None


def serve(args):
    info_path = Path(args.info)
    if info_path.exists():
        raise ValueError("Use a new private info file")
    info_path.parent.mkdir(parents=True, exist_ok=True)
    capability = secrets.token_hex(32)
    with BoundedServer(capability) as server:
        info = {
            "pid": os.getpid(), "port": server.server_port,
            "capability": capability, "max_bytes": MAX_BYTES,
            "expires_at_unix": int(time.time()) + args.lifetime,
            "running": True,
        }
        with info_path.open("x", encoding="utf-8") as output:
            json.dump(info, output)
        timer = threading.Timer(args.lifetime, server.shutdown)
        timer.daemon = True
        timer.start()
        print(json.dumps({"state": "listening", "port": server.server_port,
                          "lifetime_seconds": args.lifetime}), flush=True)
        try:
            server.serve_forever(poll_interval=0.2)
        except KeyboardInterrupt:
            pass
        finally:
            timer.cancel()
            # Only update this run's record; preserve a concurrently replaced file.
            if json.loads(info_path.read_text(encoding="utf-8")) == info:
                info["running"] = False
                info_path.write_text(json.dumps(info), encoding="utf-8")
    print(json.dumps({"state": "stopped"}), flush=True)


def probe(args):
    info = json.loads(Path(args.info).read_text(encoding="utf-8"))
    capability = info["capability"]
    if not re.fullmatch(r"[0-9a-f]{64}", capability):
        raise ValueError("Invalid probe capability")
    if not info.get("running") or info["expires_at_unix"] <= time.time():
        raise ValueError("Probe has stopped or expired")
    origin = args.origin or f"http://127.0.0.1:{info['port']}"
    uri = urlsplit(origin)
    if (uri.scheme != "https" and not (
        uri.scheme == "http" and uri.hostname == "127.0.0.1"
    )) or not uri.hostname or uri.username or uri.password or uri.query or (
        uri.fragment or uri.path not in ("", "/")
    ):
        raise ValueError("Use an HTTPS origin, or the local loopback HTTP origin")
    prefix = origin.rstrip("/") + "/" + capability + "/"
    opener = build_opener(ProxyHandler({}), NoRedirect())
    results = []

    def request(label, endpoint, data=None):
        started = time.perf_counter()
        req = Request(prefix + endpoint, data=data,
                      method="GET" if data is None else "PUT",
                      headers={"Accept-Encoding": "identity",
                               "Content-Type": "application/octet-stream"})
        with opener.open(req, timeout=90) as response:
            body = response.read(MAX_BYTES + 1)
            if len(body) > MAX_BYTES:
                raise ValueError("Response exceeded probe budget")
            results.append({"operation": label, "status": response.status,
                            "seconds": round(time.perf_counter() - started, 6),
                            "bytes": len(body) if data is None else len(data)})
            return response.headers, body

    for _ in range(3):
        _, body = request("health", "health")
        if json.loads(body) != MARKER:
            raise ValueError("Unexpected probe service")
    headers, body = request("download", f"download-{args.bytes}")
    if len(body) != args.bytes or hashlib.sha256(body).hexdigest() != (
        headers.get("X-Content-SHA256")
    ):
        raise ValueError("Download integrity mismatch")
    payload = secrets.token_bytes(args.bytes)
    _, body = request("upload", "upload", payload)
    if json.loads(body) != {"bytes": args.bytes,
                            "sha256": hashlib.sha256(payload).hexdigest()}:
        raise ValueError("Upload integrity mismatch")
    print(json.dumps({"result": "PASS", "scope": "synthetic_transport_only",
                      "tls_verified": uri.scheme == "https", "samples": results},
                     indent=2))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="mode", required=True)
    server = sub.add_parser("serve")
    server.add_argument("--info", required=True)
    server.add_argument("--lifetime", type=int, default=300)
    client = sub.add_parser("probe")
    client.add_argument("--info", required=True)
    client.add_argument("--origin")
    client.add_argument("--bytes", type=int, default=128 * 1024)
    args = parser.parse_args()
    if args.mode == "serve" and not 30 <= args.lifetime <= 900:
        parser.error("Lifetime must be between 30 and 900 seconds")
    if args.mode == "probe" and not 1 <= args.bytes <= MAX_BYTES:
        parser.error("Payload must be between 1 byte and 6 MiB")
    try:
        serve(args) if args.mode == "serve" else probe(args)
        return 0
    except HTTPError as error:
        print(json.dumps({"result": "FAIL", "kind": "HTTPError",
                          "status": error.code}))
    except (OSError, ValueError, KeyError) as error:
        # Do not print exceptions that may contain the capability-bearing URL.
        print(json.dumps({"result": "FAIL", "kind": type(error).__name__}))
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
