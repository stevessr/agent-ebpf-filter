import os
from pathlib import Path
ROOT=Path(__file__).resolve().parent

def old_round():
 candidates=([Path(os.environ['ROUND1_DIR']).resolve()] if os.environ.get('ROUND1_DIR') else [])+[ROOT.parent/'public-risk-20261017',ROOT.parent/'public-risk-experiment']
 for p in candidates:
  if (p/'prepare.py').exists():return p
 raise FileNotFoundError('Prepare round one data first, or set ROUND1_DIR')
