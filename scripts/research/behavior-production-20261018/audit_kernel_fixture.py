"""Narrow parser for CI strace fixtures; not a general strace ingestion adapter.
Only successful, complete lines and our own fixture file paths are accepted.
All retained records remain benign fixture evidence, never attack labels.
"""
import argparse,json,pathlib,re,subprocess
from collections import Counter
p=argparse.ArgumentParser();p.add_argument('--trace',type=pathlib.Path,required=True);p.add_argument('--root',type=pathlib.Path,required=True);p.add_argument('--extractor',required=True);p.add_argument('--out',type=pathlib.Path,required=True);args=p.parse_args()
root=str(args.root.resolve())+'/'
records=[];counts=Counter()
for line in args.trace.read_text(errors='strict').splitlines():
    m=re.match(r'^\s*(\d+)\s+(read|write)\(\d+<([^>]+)>.*?,\s*(\d+)\)\s+=\s+(\d+)\s*$',line)
    if m and m[3].startswith(root):
        records.append({'source':'strace_ci_kernel_syscall','syscall':{'name':m[2],'fd_path':m[3],'path_truncated':False,'requested_bytes':int(m[4]),'return':int(m[5])}});counts[m[2]]+=1;continue
    m=re.match(r'^\s*(\d+)\s+openat\(AT_FDCWD, "([^"]+)", .*\)\s+=\s+(\d+)(?:<[^>]+>)?\s*$',line)
    if m and m[2].startswith(root):
        records.append({'source':'strace_ci_kernel_syscall','syscall':{'name':'openat','dirfd':-100,'path':m[2],'path_truncated':False,'return':int(m[3])}});counts['openat']+=1;continue
    m=re.match(r'^\s*(\d+)\s+execve\("([^"]+)",.*\)\s+=\s+0\s*$',line)
    if m:
        records.append({'source':'strace_ci_kernel_syscall','syscall':{'name':'execve','path':m[2],'path_truncated':False,'return':0}});counts['execve']+=1
assert all(counts[k]>0 for k in ['read','write','openat','execve']),dict(counts)
http=json.loads((args.root/'http-observed.json').read_text());records.append({'source':'loopback_fixture_http_server','http':http})
# HTTP server observation and kernel syscalls are deliberately separate sources;
# missing process identity stays missing. No invented cross-source causal join.
result=subprocess.run([args.extractor],input=''.join(json.dumps(r)+'\n' for r in records),capture_output=True,text=True,check=True)
clean=[json.loads(line) for line in result.stdout.splitlines()];assert len(clean)==len(records)
for row in clean:
    assert row['schema']=='kernel-http-behavior.v2'
    assert not row['features']['history.30s.prior_events']['present']
assert all(root not in json.dumps(row) for row in clean)
report={'source':'actual strace syscall observations of a controlled benign CI fixture','kernel_bpf_pipeline_tested':False,'kernel_trace_counts':dict(counts),'http_observations':1,'cleaned_records':len(clean),'privacy_label':0,'destruction_label':0,'label_scope':'only authorized dummy-file/loopback fixture; not a real attack benchmark','stable_identity_missing':True,'cross_source_join_performed':False}
args.out.write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
