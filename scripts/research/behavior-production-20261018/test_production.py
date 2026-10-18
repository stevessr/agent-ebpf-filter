import importlib.util, json, pathlib, re, subprocess, sys, unittest
old=pathlib.Path(__file__).parent.parent/'behavior-features-20261018'
spec=importlib.util.spec_from_file_location('adapter',old/'adapt_capture.py');adapter=importlib.util.module_from_spec(spec);spec.loader.exec_module(adapter)
from train_production import select_threshold, metrics, http
import numpy as np
class ProductionTests(unittest.TestCase):
    def test_export_wrapper_and_protojson(self):
        for record in [{'Event':{'type':'open','retval':'0'}},{'Envelope':{'legacyEvent':{'type':'open','retval':'0'}}}]:
            o,status=adapter.adapt(record,'archive');self.assertEqual(status,'syscall');self.assertEqual(o['syscall']['return'],0)
        o,status=adapter.adapt({'Envelope':{'legacyEvent':{'type':'network','httpMethod':'GET','httpPath':'/','kernelTimestampNs':'123'}}},'archive')
        self.assertEqual(status,'http_request');self.assertEqual(o['context']['event_time_ns'],123)
    def test_alias_and_number_rejected(self):
        for r in [{'Event':{},'event':{}},{'type':'read','retval':1.2},{'type':'read','retval':True},{'type':'read','retval':2**70},{'httpMethod':'GET','http_method':'POST'}]:
            with self.assertRaises(ValueError):adapter.adapt(r,'archive')
    def test_bounded_recovery(self):
        payload=b'x'*600000+b'\n'+json.dumps({'Event':{'type':'read','retval':0}}).encode()+b'\n'
        r=subprocess.run([sys.executable,str(old/'adapt_capture.py'),'--format','archive'],input=payload,capture_output=True)
        self.assertEqual(r.returncode,2);self.assertEqual(len(r.stdout.splitlines()),1)
        self.assertEqual(json.loads(r.stderr)['audit']['oversized'],1)
    def test_duplicate_and_nonfinite_json_rejected(self):
        r=subprocess.run([sys.executable,str(old/'adapt_capture.py'),'--format','archive'],input=b'{"type":"read","retval":0,"retval":1}\n{"type":"read","retval":NaN}\n',capture_output=True)
        self.assertEqual(r.returncode,2);self.assertEqual(r.stdout,b'')
    def test_threshold_requires_negative_support(self):
        _,ok=select_threshold(np.array([0]*10+[1]*10),np.linspace(0,1,20));self.assertFalse(ok)
        y=np.array([0]*1000+[1]*1000);scores=np.r_[np.linspace(0,.2,1000),np.linspace(.3,.9,1000)]
        t,ok=select_threshold(y,scores);self.assertTrue(ok);self.assertLessEqual(metrics(y,scores,t)['fpr_upper95'],.01)
    def test_strace_openat_annotated_dirfd(self):
        # Test the actual regex embedded in the deliberately narrow CI parser.
        source=(pathlib.Path(__file__).parent/'audit_kernel_fixture.py').read_text()
        pattern=next(line.split("re.match(r'",1)[1].split("',line)",1)[0] for line in source.splitlines() if 'openat\\(' in line)
        for fd in ['AT_FDCWD','AT_FDCWD</tmp/runner>']:
            m=re.match(pattern,'123 openat('+fd+', "/tmp/fixture/input.json", O_RDONLY|O_CLOEXEC) = 4</tmp/fixture/input.json>')
            self.assertIsNotNone(m)
            self.assertEqual(m[2],'/tmp/fixture/input.json')
    def test_request_parser_preserves_headers(self):
        h=http('POST /a?x=1 HTTP/1.1\r\nX-A: 1\r\nX-A: 2\r\n\r\na=hello')
        self.assertEqual(len(h['headers']),2);self.assertEqual(h['body'],'a=hello')
if __name__=='__main__':unittest.main()
