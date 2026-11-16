"""Throttled, interruptible warming through existing protected HTTPS routes."""
import argparse
import json
import re
import signal
import ssl
import stat
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

UUID = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\Z")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def origin(value):
    parsed = urllib.parse.urlsplit(value)
    if (parsed.scheme != "https" or not parsed.hostname or parsed.username
            or parsed.password or parsed.path not in ("", "/")
            or parsed.query or parsed.fragment):
        raise ValueError("an HTTPS origin is required")
    return value.rstrip("/")


def thumbnail_paths(post, group):
    if not isinstance(group, dict) or group.get("postId") != post:
        raise ValueError("invalid manifest")
    items = group.get("images")
    if not isinstance(items, list) or len(items) > 9:
        raise ValueError("invalid manifest")
    seen, paths = set(), []
    for item in items:
        content = item.get("contentVersionId", "")
        if not isinstance(content, str) or not UUID.fullmatch(content) or content in seen:
            raise ValueError("invalid content version")
        seen.add(content)
        path = f"/api/v1/posts/{post}/images/{content}/thumbnail"
        if item.get("thumbnail", {}).get("path") != path:
            raise ValueError("thumbnail-v1 is unavailable")
        paths.append(path)
    return paths


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--origin", required=True)
    parser.add_argument("--token-file", type=Path, required=True)
    parser.add_argument("--post-ids", type=Path, required=True, help="one post UUID per line")
    parser.add_argument("--ca-file", help="optional private CA PEM; TLS verification stays enabled")
    parser.add_argument("--interval", type=float, default=1.0, help="minimum seconds between requests")
    parser.add_argument("--max-posts", type=int, default=100)
    args = parser.parse_args()
    base = origin(args.origin)
    if args.interval < 0.1 or args.interval > 3600 or not 1 <= args.max_posts <= 10000:
        parser.error("interval must be 0.1..3600 and max-posts 1..10000")
    if stat.S_IMODE(args.token_file.stat().st_mode) & 0o077 and __import__("os").name != "nt":
        parser.error("token file must be private (0600)")
    if args.token_file.stat().st_size > 128:
        parser.error("invalid token file")
    token = args.token_file.read_text(encoding="ascii").strip()
    if not re.fullmatch(r"[A-Za-z0-9_-]{43}", token):
        parser.error("invalid token file")
    # No proxy credentials or redirects are inherited. Bytes are discarded and
    # never persisted; this tool has precisely the caller's current permissions.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect(),
        urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=args.ca_file)))
    stopped = False
    stop_event = threading.Event()
    def stop(*_):
        nonlocal stopped
        stopped = True
        stop_event.set()
    signal.signal(signal.SIGINT, stop)
    signal.signal(signal.SIGTERM, stop)
    last = 0.0
    def get(path, maximum):
        nonlocal last
        remaining = args.interval - (time.monotonic() - last)
        if remaining > 0:
            stop_event.wait(remaining)
        if stopped:
            raise InterruptedError()
        last = time.monotonic()
        request = urllib.request.Request(base + path, headers={
            "Authorization": "Bearer " + token, "X-Post-Preview": "thumbnail-v1"})
        with opener.open(request, timeout=15) as response:
            if response.headers.get("Cache-Control") != "no-store":
                raise ValueError("unexpected cache policy")
            data = response.read(maximum + 1)
            if len(data) > maximum:
                raise ValueError("response budget exceeded")
            return data
    result = {"posts": 0, "thumbnails": 0, "unavailable": 0, "stopped": False}
    try:
        with args.post_ids.open(encoding="ascii") as lines:
            for line in lines:
                if stopped or result["posts"] >= args.max_posts:
                    break
                post = line.strip()
                if not UUID.fullmatch(post):
                    raise ValueError("invalid post ID list")
                result["posts"] += 1
                try:
                    group = json.loads(get(f"/api/v1/posts/{post}/images", 32768))
                    for path in thumbnail_paths(post, group):
                        get(path, 512 * 1024)
                        result["thumbnails"] += 1
                except urllib.error.HTTPError as error:
                    if error.code in (401, 403, 429, 503):
                        result["stopped"] = True
                        break
                    if error.code != 404:
                        raise ValueError("unexpected HTTP status") from None
                    result["unavailable"] += 1
    except InterruptedError:
        stopped = True
    result["stopped"] = result["stopped"] or stopped
    print(json.dumps(result, separators=(",", ":")))
    return 2 if result["stopped"] else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, KeyError, TypeError):
        # Do not expose tokens, URLs, resource IDs or server error bodies.
        raise SystemExit("thumbnail warming failed") from None
