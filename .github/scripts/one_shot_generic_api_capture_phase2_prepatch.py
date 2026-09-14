from pathlib import Path

path = Path("backend/app/tls/api_fingerprint.go")
text = path.read_text()
old = '''\tevent.APIConfidence = match.Confidence\n\t// A host-specific fingerprint is stronger than the historical substring\n\t// vendor inference. Path-only compatible profiles only fill an empty value.\n\tif event.Vendor == "" || !strings.HasSuffix(match.Vendor, "-compatible") {\n'''
new = '''\tevent.APIConfidence = match.Confidence\n\tif event.Vendor == "" || !strings.HasSuffix(match.Vendor, "-compatible") {\n'''
if old not in text:
    raise SystemExit("missing current TLS fingerprint comment anchor")
path.write_text(text.replace(old, new, 1))
