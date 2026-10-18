import os
from pathlib import Path
ROOT=Path(__file__).resolve().parent
def resolve(env,candidates):
    for p in ([Path(os.environ[env])] if os.environ.get(env) else [])+candidates:
        if p.exists():return p.resolve()
    raise FileNotFoundError(env)
R2=resolve('ROUND2_DIR',[ROOT.parent/'public-risk-20261018',ROOT.parent/'public-risk-round2'])
R1=resolve('ROUND1_DIR',[ROOT.parent/'public-risk-20261017',ROOT.parent/'public-risk-experiment'])
