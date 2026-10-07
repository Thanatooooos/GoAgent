import json
import os
import subprocess
import tempfile
from http.server import BaseHTTPRequestHandler, HTTPServer


MAX_IMAGE_BYTES = 10 * 1024 * 1024
LANGUAGES = os.environ.get("OCR_LANGUAGES", "chi_sim+eng")


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/health":
            self.send_error(404)
            return
        self.respond(200, {"status": "ok"})

    def do_POST(self):
        if self.path != "/recognize":
            self.send_error(404)
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            self.respond(400, {"error": "invalid content length"})
            return
        if length <= 0 or length > MAX_IMAGE_BYTES:
            self.respond(413, {"error": "image size is out of range"})
            return
        image = self.rfile.read(length)
        if len(image) != length:
            self.respond(400, {"error": "incomplete image"})
            return
        with tempfile.NamedTemporaryFile(suffix=".img") as source:
            source.write(image)
            source.flush()
            try:
                result = subprocess.run(
                    ["tesseract", source.name, "stdout", "-l", LANGUAGES],
                    capture_output=True,
                    text=True,
                    timeout=30,
                    check=False,
                )
            except subprocess.TimeoutExpired:
                self.respond(504, {"error": "OCR timed out"})
                return
        if result.returncode != 0:
            self.respond(422, {"error": result.stderr.strip()[:500]})
            return
        text = result.stdout.strip()
        self.respond(200, {"status": "success" if text else "no_text", "text": text})

    def respond(self, status, payload):
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


HTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
