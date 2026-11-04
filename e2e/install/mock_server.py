#!/usr/bin/env python3
"""Server tiruan untuk menguji install.sh: menyajikan /api/v1/agent/download dari berkas di direktori (bin, ver, sig, sha)."""
import http.server, os, ssl, sys

root, port = sys.argv[1], int(sys.argv[2])
cert = sys.argv[3] if len(sys.argv) > 3 else None
key = sys.argv[4] if len(sys.argv) > 4 else None


def read(name, mode="r"):
    with open(os.path.join(root, name), mode) as f:
        return f.read()


class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_GET(self):
        if self.path == "/ca.pem" and os.path.exists(os.path.join(root, "ca.pem")):
            body = read("ca.pem", "rb")
            self.send_response(200); self.send_header("Content-Type", "application/x-pem-file")
            self.send_header("Content-Length", str(len(body))); self.end_headers(); self.wfile.write(body); return
        if self.path != "/api/v1/agent/download" or self.headers.get("Authorization") != "Bearer oen_testtoken":
            self.send_response(401); self.end_headers(); return
        body = read("bin", "rb")
        self.send_response(200)
        self.send_header("Content-Type", "application/octet-stream")
        self.send_header("X-Release-Version", read("ver").strip())
        self.send_header("X-Sha256", read("sha").strip())
        self.send_header("X-Signature-Ed25519", read("sig").strip())
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


class S(http.server.ThreadingHTTPServer):
    def handle_error(self, request, client_address):
        pass  # klien yang menolak TLS memutus koneksi; itu bukan galat server tiruan


srv = S(("127.0.0.1", port), H)
if cert:
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.load_cert_chain(cert, key)
    srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
srv.serve_forever()
