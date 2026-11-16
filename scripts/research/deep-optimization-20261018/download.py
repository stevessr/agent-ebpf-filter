"""Pinned public assets only. Dataset strings are never executed or requested."""
import argparse,concurrent.futures,hashlib,json,urllib.request
from pathlib import Path

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--assets',type=Path,required=True);ap.add_argument('--data',type=Path,required=True);ap.add_argument('--root',type=Path,required=True);ap.add_argument('--evaluation',action='store_true');a=ap.parse_args();jobs=[]
    align='https://huggingface.co/datasets/ServiceNow/PrivacyAlign/resolve/edaf507a2d1019b03b592b92a60d113b7eb46425/'
    for s in ['train','test']:jobs.append((a.assets/'raw'/f'PrivacyAlign_{s}.jsonl',align+f'data/{s}.jsonl',False))
    jobs.append((a.assets/'raw/PrivacyAlign_README.md',align+'README.md',False))
    for directory,repo,revision,weight in [(a.assets/'base_model','google/bert_uncased_L-2_H-128_A-2','30b0a37ccaaa32f332884b96992754e246e48c5f','model.safetensors'),(a.root/'base_mini','google/bert_uncased_L-4_H-256_A-4','387825ce42dbb39b87911cdf8e383ee3b25184f8','pytorch_model.bin')]:
        for f in ['config.json','vocab.txt',weight,'README.md']:jobs.append((directory/f,f'https://huggingface.co/{repo}/resolve/{revision}/{f}',False))
    revision='424c6e1c1951570473d9d6c0227eaaac6fdd6d02';paths=['OriginalDataSets/csic_2010/'+f for f in ['normalTrafficTraining.txt','normalTrafficTest.txt','anomalousTrafficTest.txt','README.md']]+['OriginalDataSets/ecml_pkdd/dataset_ecml_pkdd_train_test/'+f for f in ['xml_train.txt','xml_test.txt']]+['LICENSE','README.md','OriginalDataSets/ecml_pkdd/README.md']
    for f in paths:jobs.append((a.data/f,f'https://raw.githubusercontent.com/msudol/Web-Application-Attack-Datasets/{revision}/{f}',False))
    if a.evaluation:
        # Exact evaluation selections are checked in alongside the report.
        for corpus in ['pisas','crs','crs_v2']:
            protocol_path=Path(__file__).resolve().parent/f'{corpus}_protocol.json';protocol=json.loads(protocol_path.read_text());directory=a.root/corpus;directory.mkdir(parents=True,exist_ok=True);(directory/'protocol.json').write_bytes(protocol_path.read_bytes())
            if corpus=='pisas':
                files=[f+'/'+n for f in protocol['folders'] for n in ['scenario.json','appropriateness.json','visibility.json']];base=f'https://huggingface.co/datasets/ServiceNow/PiSAs/resolve/{protocol["revision"]}/'
            else:files=protocol['files'];base=f'https://raw.githubusercontent.com/coreruleset/coreruleset/{protocol["revision"]}/'
            for f in files:jobs.append((directory/f,base+f,corpus.startswith('crs')))
    expected=json.loads((Path(__file__).resolve().parent/'source_hashes.json').read_text())
    def fetch(item):
        p,u,optional=item;p.parent.mkdir(parents=True,exist_ok=True)
        if not p.exists():
            try:
                with urllib.request.urlopen(u,timeout=180) as response:p.write_bytes(response.read())
            except urllib.error.HTTPError as e:
                if optional and e.code==404:return dict(path=str(p),status='predeclared file unavailable')
                raise
        digest=hashlib.sha256(p.read_bytes()).hexdigest()
        if u in expected and digest!=expected[u]:raise ValueError('pinned public source checksum mismatch: '+str(p))
        return dict(path=str(p),sha256=digest,bytes=p.stat().st_size)
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as ex:manifest=list(ex.map(fetch,jobs))
    a.root.mkdir(parents=True,exist_ok=True);(a.root/'downloads.json').write_text(json.dumps(manifest,indent=2)+'\n');print(json.dumps(manifest,indent=2))
if __name__=='__main__':main()
