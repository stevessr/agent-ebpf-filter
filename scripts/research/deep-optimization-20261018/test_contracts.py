"""Security-relevant representation contracts, independent of trained verdicts."""
import unittest
from corrected import target,text,go_features
from pathlib import Path
import os

class Contracts(unittest.TestCase):
    def test_literal_fragment_is_request_data(self):
        raw='/search?query=first#second%23third'
        self.assertEqual(target(raw),raw)
        self.assertIn('#second%23third',text(dict(http=dict(target=raw,body=''))))
    def test_absolute_authority_normalization_preserves_suffix(self):
        self.assertEqual(target('http://example.invalid:8080/search?q=1#suffix'),'/search?q=1#suffix')
        self.assertEqual(target('https://example.invalid?q=1#suffix'),'/?q=1#suffix')
    def test_body_spaces_are_data(self):
        self.assertEqual(text(dict(http=dict(target='/echo',body='  hello  \r\n'))),'/echo\n  hello  ')
    @unittest.skipUnless(os.environ.get('BEHAVIOR_EXTRACTOR'),'requires actual Go cleaner')
    def test_body_ratio_has_body_denominator_and_presence_is_retained(self):
        e=dict(http=dict(method='POST',target='/short',headers=[],headers_present=True,headers_preserve_duplicates=True,body='"abc"',truncated=False,direction='outgoing'))
        x,n=go_features([e],Path(os.environ['BEHAVIOR_EXTRACTOR']));idx={name:i for i,name in enumerate(n)}
        self.assertAlmostEqual(x[0,idx['per_body_byte:body.quote_count']],2/5,places=6)
        self.assertEqual(x[0,idx['present:sys.actual_bytes']],0)
        self.assertEqual(x[0,idx['present:body.quote_count']],1)
if __name__=='__main__':unittest.main()
