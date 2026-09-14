from pathlib import Path

path = Path('.github/scripts/one_shot_generic_api_capture.py')
text = path.read_text()
old = 'Vendor:         event.GetServiceName()'
new = 'Vendor:         platform.FirstNonEmpty(event.GetServiceName(), platform.ParseStringField(event.GetExtraInfo(), "vendor"))'
count = text.count(old)
if count != 2:
    raise SystemExit(f'expected two staged TLS vendor literals, got {count}')
path.write_text(text.replace(old, new))
