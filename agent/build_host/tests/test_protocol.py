import base64
import hashlib
import json
import unittest
from pathlib import Path
from common import host

class ProtocolTests(unittest.TestCase):
    def test_shared_vectors(self):
        for v in json.loads((Path(__file__).resolve().parents[3] / 'internal/auth/testdata/host_signature_vectors.json').read_text()):
            body = v['body'].encode()
            self.assertEqual(host.signature_message(v['method'], v['path'], v['timestamp'], body).decode(), v['message'])
            key = host.SigningKey(bytes.fromhex(v['seed']))
            headers = host.signed_headers(7, key, v['method'], v['path'], body, int(v['timestamp']))
            self.assertEqual(headers['X-Signature'], v['signature'])
            self.assertEqual(base64.b64encode(bytes(key.verify_key)).decode(), v['public_key'])
            self.assertEqual(headers['X-Build-Host'], '7')

    def test_origin_validation(self):
        self.assertEqual(host.board_origin('http://127.0.0.1:8080/'), 'http://127.0.0.1:8080')
        for value in ['http://example.org', 'https://u:p@example.org', 'https://example.org/path', 'https://example.org?x=1', 'file:///tmp', 'https://example.org/#frag']:
            with self.subTest(value=value), self.assertRaises(ValueError):
                host.board_origin(value)

    def test_redirects_are_not_followed(self):
        import http.server
        import threading
        calls=[]
        class Redirect(http.server.BaseHTTPRequestHandler):
            def do_POST(self):
                calls.append(self.path)
                self.send_response(307)
                self.send_header('Location','/destination')
                self.end_headers()
            def log_message(self,*args): pass
        server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Redirect)
        thread=threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            board=host.BoardClient('http://127.0.0.1:'+str(server.server_port))
            with self.assertRaises(host.BoardError): board.request('POST',host.API+'/pair',{},signed=False)
            self.assertEqual(calls,[host.API+'/pair'])
        finally:
            server.shutdown();server.server_close();thread.join()
