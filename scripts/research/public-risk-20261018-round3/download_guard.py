import json,hashlib,requests
from pathlib import Path
from paths import ROOT
REV='56988fbffdd37d4b0f477066457ddab8b197b3dc'
def main():
    root=ROOT/'models/astrlink-guard';root.mkdir(parents=True,exist_ok=True);manifest={}
    files=['README.md','LICENSE','licenses/SOURCES.json','int8/model_int8.onnx','int8/model_int8.onnx_data','int8/tokenizer.json','int8/config.json','int8/labels.json','int8/CHECKSUMS.json']
    for f in files:
        p=root/f;p.parent.mkdir(parents=True,exist_ok=True)
        if not p.exists():
            r=requests.get(f'https://huggingface.co/QuantumNous/astrlink-guard/resolve/{REV}/{f}',timeout=90);r.raise_for_status();p.write_bytes(r.content)
        manifest[f]={'bytes':p.stat().st_size,'sha256':hashlib.sha256(p.read_bytes()).hexdigest()}
        print('downloaded',f,p.stat().st_size,flush=True)
    assert manifest['int8/model_int8.onnx']['sha256']=='0a070d3113615e33ba3b84d6bd486f663a377c04c4b056e7381732179f51832a'
    assert manifest['int8/model_int8.onnx_data']['sha256']=='0ba34216ceead01e0ae2dfd8e46d52b67e7aee80b04f722d9b758613a91c0067'
    (ROOT/'artifacts/guard_assets.json').write_text(json.dumps({'revision':REV,'repo':'QuantumNous/astrlink-guard','files':manifest},indent=2))
if __name__=='__main__':main()
