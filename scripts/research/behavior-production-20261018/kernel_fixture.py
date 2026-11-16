"""Consent-scoped benign fixture. Run only in an isolated CI/temp workspace.
It reads/writes its own dummy files, runs a harmless child and sends one request
only to its own loopback HTTP server. No production files or credentials.
"""
import http.server,json,pathlib,subprocess,sys,threading,urllib.request
root=pathlib.Path(sys.argv[1]);root.mkdir(mode=0o700,exist_ok=True)
(root/'input.json').write_text('{"items":[3,1,2]}')
data=json.loads((root/'input.json').read_text());(root/'output.json').write_text(json.dumps(sorted(data['items'])))
class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body=self.rfile.read(int(self.headers.get('Content-Length',0)))
        if body!=b'{"fixture":true}':raise ValueError('unexpected fixture body')
        (root/'http-observed.json').write_text(json.dumps({'method':'POST','target':self.path,'body':body.decode(),'headers':[{'name':k,'value':v} for k,v in self.headers.items()],'headers_present':True,'headers_preserve_duplicates':True,'truncated':False,'direction':'outgoing'}))
        self.send_response(200);self.end_headers();self.wfile.write(b'ok')
    def log_message(self,*args):pass
server=http.server.HTTPServer(('127.0.0.1',0),Handler);thread=threading.Thread(target=server.handle_request);thread.start()
request=urllib.request.Request(f'http://127.0.0.1:{server.server_port}/fixture',data=b'{"fixture":true}',headers={'Content-Type':'application/json'})
# Explicitly bypass ambient proxy configuration for this loopback-only fixture.
opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
with opener.open(request,timeout=5) as response:assert response.read()==b'ok'
thread.join(timeout=5);assert not thread.is_alive();server.server_close()
subprocess.run([sys.executable,'-I','-c','print(1+1)'],check=True,stdout=subprocess.DEVNULL)
