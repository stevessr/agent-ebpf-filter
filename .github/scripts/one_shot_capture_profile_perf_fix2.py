from pathlib import Path

source_path = Path(__file__).with_name("one_shot_capture_profile_perf_fix.py")
source = source_path.read_text()
start = source.index("# read(2) incoming pair.")
end = source.index("# write(2) outgoing pair.", start)
source = source[:start] + source[end:]
exec(compile(source, str(source_path), "exec"))
