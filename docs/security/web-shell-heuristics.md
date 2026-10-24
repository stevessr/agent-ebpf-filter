# Web-service shell-spawn heuristics

This heuristic detects OS-shell processes launched directly from positively identified PHP, Python, or Node.js/TypeScript web workers.

## Rule codes

| Code | Positive parent evidence |
| --- | --- |
| PHP_WEB_SHELL_SPAWN | php-fpm, php-cgi, PHP built-in HTTP server, Laravel artisan serve |
| PYTHON_WEB_SHELL_SPAWN | uvicorn, gunicorn, hypercorn, daphne, waitress, granian, Flask, FastAPI, Django runserver, Python http.server |
| JS_WEB_SHELL_SPAWN | Node.js, Bun, Deno, tsx, ts-node with a server entrypoint; Next, Nuxt, Nest HTTP service commands |

Targets include sh, bash, dash, ash, zsh, fish, ksh, mksh, csh, tcsh and pwsh, plus explicit env/busybox shell invocations. Shell names must be exact, not substrings (shellcheck is not a shell).

## Examples

- php-fpm: pool www launching /bin/sh: PHP_WEB_SHELL_SPAWN
- python -m uvicorn main:app launching sh: PYTHON_WEB_SHELL_SPAWN
- tsx src/server.ts launching bash: JS_WEB_SHELL_SPAWN
- python -m pytest launching bash: no web-service match
- node scripts/build.js launching sh: no web-service match
- php artisan queue:work launching sh: no web-service match

Only interpreter names are insufficient evidence. An unknown Python app.py or Node index.js is not presumed to serve HTTP. A bounded procfs command-line lookup identifies the immediate parent. Missing procfs data fails to *no match*, rather than silently categorizing arbitrary code as a web service.

## Enforcement boundary

- Commands passing through agent-wrapper receive BLOCK from resolveAction before ML triage when this heuristic matches. The trusted health-generator exception cannot bypass this rule.
- Kernel-observed execve, execveat and process_exec events yield a high-risk semantic alert (98/100), but do not themselves prevent the syscall.
- Unwrapped shell execution is NOT blocked by this change. Enforcing that boundary universally requires an explicit kernel LSM policy based on trustworthy process ancestry or a restrictive sandbox.

Procfs attribution can fail for fast-exiting processes and remains subject to PID reuse. These rules are conservative and deterministic to avoid preventing legitimate test, build, and maintenance scripts.
