#!/usr/bin/env python3
"""PROTOTYPE: serve a disposable browser companion to the Gio workspace."""
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import argparse

HERE = Path(__file__).resolve().parent
parser = argparse.ArgumentParser()
parser.add_argument('--port', type=int, default=8766)
args = parser.parse_args()

class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(HERE), **kwargs)

    def do_GET(self):
        resources = {'/tokens.json': HERE / 'baseline/tokens.json',
                     '/font.otf': HERE.parent / 'internal/uifont/assets/NotoSansSC-Regular.otf'}
        path = self.path.split('?')[0]
        if path in resources:
            data = resources[path].read_bytes()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json' if path.endswith('.json') else 'font/otf')
            self.send_header('Content-Length', str(len(data)))
            self.end_headers()
            self.wfile.write(data)
            return
        super().do_GET()

print(f'Configuration prototype: http://127.0.0.1:{args.port}/?variant=A', flush=True)
ThreadingHTTPServer(('127.0.0.1', args.port), Handler).serve_forever()
