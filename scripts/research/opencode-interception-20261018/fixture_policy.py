"""Research canary policy, NOT the project's learned model or production engine.
A deliberately narrow literal marker deny demonstrates execution-hook wiring.
It is bypassable and is not a shell parser, sandbox, or production security rule.
"""
import json, sys
raw=sys.stdin.buffer.read(256*1024+1)
if len(raw)>256*1024:raise SystemExit(2)
request=json.loads(raw)
# This policy is fixed before runs. No ground-truth label or case id is read.
text=json.dumps(request.get('args',{}),ensure_ascii=True)
action='BLOCK' if 'protected-canary' in text else 'ALLOW'
print(json.dumps({'action':action}))
