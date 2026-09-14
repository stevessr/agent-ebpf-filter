from pathlib import Path

path = Path("backend/app/jobs_background.go")
text = path.read_text()
text = text.replace('\t"strings"\n', '', 1)
path.write_text(text)
