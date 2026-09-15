#!/usr/bin/env python3
"""Verify a local runtime container under OpenShift-like restrictions."""

import json
import subprocess
import sys
import time
import urllib.error
import urllib.request


def check(base, path, method, status, content_type):
    request = urllib.request.Request(base + path, method=method)
    try:
        response = urllib.request.urlopen(request, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        assert response.status == status, (path, response.status)
        assert content_type in response.headers.get("Content-Type", ""), path
        body = response.read()
        if path == "/healthz":
            assert json.loads(body) == {"status": "ok"}
        if method == "HEAD":
            assert body == b""
    print(f"{method} {path}: {status} OK")


def main():
    image = sys.argv[1]
    container = subprocess.check_output([
        "docker", "run", "-d", "--read-only", "--user", "1000780000:0",
        "--tmpfs", "/data:rw,noexec,nosuid,size=64m,mode=0770,uid=1000780000,gid=0",
        "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
        "-p", "127.0.0.1::8080", image,
    ], text=True).strip()
    try:
        address = subprocess.check_output(["docker", "port", container, "8080"], text=True).strip()
        base = "http://" + address
        for attempt in range(30):
            try:
                with urllib.request.urlopen(base + "/healthz", timeout=2) as response:
                    assert response.status == 200
                break
            except OSError:
                if attempt == 29:
                    raise
                time.sleep(0.2)
        for case in [
            ("/", "GET", 200, "text/html"),
            ("/style.css", "GET", 200, "text/css"),
            ("/healthz", "GET", 200, "application/json"),
            ("/api/media", "GET", 200, "application/json"),
            ("/favicon.svg", "GET", 200, "image/svg+xml"),
            ("/missing", "GET", 404, "text/plain"),
            ("/", "POST", 405, "text/plain"),
            ("/", "HEAD", 200, "text/html"),
        ]:
            check(base, *case)
        subprocess.run(["docker", "stop", "--time", "15", container], check=True, stdout=subprocess.DEVNULL)
        code = subprocess.check_output(["docker", "inspect", "--format", "{{.State.ExitCode}}", container], text=True).strip()
        assert code == "0", code
        print("Arbitrary UID, read-only filesystem and graceful SIGTERM: OK")
    finally:
        subprocess.run(["docker", "rm", "-f", container], check=True, stdout=subprocess.DEVNULL)


if __name__ == "__main__":
    main()
