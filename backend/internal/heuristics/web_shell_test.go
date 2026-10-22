package heuristics

import "testing"

func TestIdentifyWebService(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{"php-fpm master", []string{"/usr/sbin/php-fpm8.3", "-F"}, "PHP"},
		{"php-fpm worker", []string{"php-fpm: pool www"}, "PHP"},
		{"php CGI", []string{"/usr/bin/php-cgi"}, "PHP"},
		{"PHP built-in server", []string{"php", "-S", "0.0.0.0:8080", "-t", "public"}, "PHP"},
		{"Laravel server", []string{"php", "artisan", "serve"}, "PHP"},
		{"uvicorn", []string{"uvicorn", "api:app", "--host", "0.0.0.0"}, "Python"},
		{"python module uvicorn", []string{"python3", "-m", "uvicorn", "api:app"}, "Python"},
		{"gunicorn", []string{"/usr/local/bin/gunicorn", "project.wsgi:application"}, "Python"},
		{"django", []string{"python3", "manage.py", "runserver"}, "Python"},
		{"http server", []string{"python3", "-m", "http.server", "8000"}, "Python"},
		{"flask", []string{"flask", "run", "--host", "0.0.0.0"}, "Python"},
		{"python flask module", []string{"python3", "-m", "flask", "run"}, "Python"},
		{"Node server", []string{"node", "dist/server.js"}, "Node.js/TypeScript"},
		{"TypeScript server", []string{"tsx", "src/server.ts"}, "Node.js/TypeScript"},
		{"Deno server", []string{"deno", "run", "--allow-net", "server.ts"}, "Node.js/TypeScript"},
		{"Next server", []string{"next", "start"}, "Node.js/TypeScript"},
		{"Node Next server", []string{"node", "node_modules/next/dist/bin/next", "start"}, "Node.js/TypeScript"},
		{"Python CLI", []string{"python3", "-m", "pytest"}, ""},
		{"Python worker", []string{"python3", "-m", "celery", "worker"}, ""},
		{"PHP CLI", []string{"php", "script.php"}, ""},
		{"PHP syntax highlight CLI", []string{"php", "-s", "script.php"}, ""},
		{"PHP Laravel queue", []string{"php", "artisan", "queue:work"}, ""},
		{"Node build", []string{"node", "scripts/build.js"}, ""},
		{"Node test", []string{"node", "--test", "tests/a.test.js"}, ""},
		{"TypeScript CLI", []string{"tsx", "scripts/generate.ts"}, ""},
		{"TypeScript CLI with port argument", []string{"tsx", "scripts/generate.ts", "--port", "8080"}, ""},
		{"Bun package installer", []string{"bun", "install"}, ""},
		{"npm run dev not sufficient", []string{"npm", "run", "dev"}, ""},
		{"anonymous python app script", []string{"python", "app.py"}, ""},
		{"unknown", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IdentifyWebService(tt.argv); got != tt.want {
				t.Fatalf("IdentifyWebService(%q)=%q; want %q", tt.argv, got, tt.want)
			}
		})
	}
}

func TestShellExecutable(t *testing.T) {
	tests := []struct {
		command string
		args    []string
		want    string
	}{
		{"/bin/sh", []string{"-c", "id"}, "sh"},
		{"bash", []string{"-lc", "whoami"}, "bash"},
		{"pwsh", nil, "pwsh"},
		{"busybox", []string{"ash", "-c", "id"}, "ash"},
		{"/usr/bin/env", []string{"-i", "HOME=/tmp", "/bin/sh", "-c", "true"}, "sh"},
		{"/usr/bin/env", []string{"-S", "bash -lc", "id"}, "bash"},
		{"busybox", []string{"ls"}, ""},
		{"shellcheck", []string{"x.sh"}, ""},
		{"env", []string{"PATH=/usr/bin", "python3", "-c", "print('sh')"}, ""},
		{"node", []string{"shell.js"}, ""},
	}
	for _, tt := range tests {
		if got := ShellExecutable(tt.command, tt.args); got != tt.want {
			t.Errorf("ShellExecutable(%q, %q)=%q want %q", tt.command, tt.args, got, tt.want)
		}
	}
}

func TestMatchWebShellSpawn(t *testing.T) {
	tests := []struct {
		name string
		cmd string
		args []string
		parent []string
		wantCode string
	}{
		{"PHP FPM spawns sh", "/bin/sh", []string{"-c", "curl x"}, []string{"php-fpm: pool www"}, "PHP_WEB_SHELL_SPAWN"},
		{"PHP built-in server spawns bash", "bash", nil, []string{"php", "-S", "127.0.0.1:8080"}, "PHP_WEB_SHELL_SPAWN"},
		{"Python ASGI spawns sh", "sh", []string{"-c", "id"}, []string{"python3", "-m", "uvicorn", "main:app"}, "PYTHON_WEB_SHELL_SPAWN"},
		{"Node TS server spawns bash", "/bin/bash", nil, []string{"tsx", "src/server.ts"}, "JS_WEB_SHELL_SPAWN"},
		{"Node env shell", "env", []string{"/bin/sh", "-c", "id"}, []string{"node", "server.mjs"}, "JS_WEB_SHELL_SPAWN"},
		{"Python test runner spawns bash", "bash", nil, []string{"python", "-m", "pytest"}, ""},
		{"Node build spawns bash", "bash", nil, []string{"node", "scripts/build.js"}, ""},
		{"Web app launches ordinary command", "ls", nil, []string{"node", "dist/server.js"}, ""},
		{"Unknown parent", "bash", nil, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finding, ok := MatchWebShellSpawn(tt.cmd, tt.args, tt.parent, 1234)
			if (tt.wantCode != "") != ok {
				t.Fatalf("match=%v finding=%+v, want code %q", ok, finding, tt.wantCode)
			}
			if ok && (finding.Code != tt.wantCode || finding.ParentPID != 1234 || finding.Reason() == "") {
				t.Fatalf("finding=%+v, want code %q", finding, tt.wantCode)
			}
		})
	}
}
