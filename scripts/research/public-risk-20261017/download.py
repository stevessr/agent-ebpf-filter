"""Download only pinned public data/model assets used for training."""
from pathlib import Path
import concurrent.futures,urllib.request
ROOT=Path(__file__).resolve().parent
SOURCES={
 'beaver':('PKU-Alignment/BeaverTails','8401fe609d288129cc684a9b3be6a93e41cfe678'),
 'devign':('google/code_x_glue_cc_defect_detection','69bd48c03223c2104342acd9a807caf61ac3efb8'),
 'harm':('ai-safety-institute/AgentHarm','e23b3fe60a0da9037314b88e5ee3a0c054970dad'),
 'align':('ServiceNow/PrivacyAlign','edaf507a2d1019b03b592b92a60d113b7eb46425'),
 'bert':('google/bert_uncased_L-2_H-128_A-2','30b0a37ccaaa32f332884b96992754e246e48c5f'),
}
def url(k,p):
 name,rev=SOURCES[k];return f'https://huggingface.co/{"" if k=="bert" else "datasets/"}{name}/resolve/{rev}/{p}'
def jobs():
 for s in ['train','test']:yield f'raw/beaver_{s}.jsonl.gz',url('beaver',f'round0/30k/{s}.jsonl.gz')
 yield 'raw/BeaverTails_README.md',url('beaver','README.md')
 for s in ['train','validation','test']:yield f'raw/devign_{s}.parquet',url('devign',f'data/{s}-00000-of-00001.parquet')
 yield 'raw/Devign_README.md',url('devign','README.md')
 for n,p in [('harm_public.json','harmful_behaviors_test_public.json'),('harm_benign.json','benign_behaviors_test_public.json')]:yield 'raw/'+n,url('harm','benchmark/'+p)
 yield 'raw/AgentHarm_LICENSE',url('harm','LICENSE')
 for s in ['train','test']:yield f'raw/PrivacyAlign_{s}.jsonl',url('align',f'data/{s}.jsonl')
 yield 'raw/PrivacyAlign_README.md',url('align','README.md')
 for n in ['config.json','vocab.txt','model.safetensors','README.md']:yield 'base_model/'+n,url('bert',n)
def fetch(item):
 name,u=item;p=ROOT/name;p.parent.mkdir(exist_ok=True,parents=True)
 if p.exists():return
 with urllib.request.urlopen(u,timeout=180) as r:p.write_bytes(r.read())
 print(name,flush=True)
if __name__=='__main__':
 with concurrent.futures.ThreadPoolExecutor(max_workers=4) as ex:list(ex.map(fetch,jobs()))
