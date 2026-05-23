# Dev Env TUI

Standalone Go TUI editor for the repository local development environment.
It edits the same local-only files as `scripts/dev-env.sh`:

- `.env.dev` shell exports
- `.env.dev.mk` Makefile overrides

Run from the repository root:

```bash
make dev-env        # default TUI
make dev-env-build  # writes bin/dev-env-tui
```

Keyboard and mouse controls:

- Mouse click a group, field, or button to focus/activate it
- Mouse wheel scrolls long forms and preview/doctor popups
- `Ctrl+G` focus groups
- `Ctrl+F` focus the current group form
- `Ctrl+S` save `.env.dev` / `.env.dev.mk`
- `Ctrl+D` show doctor checks
- `Ctrl+P` preview values with secrets redacted
- `Ctrl+Q` quit
- `Esc` closes preview/doctor popups

Empty fields are not written. Secret-looking keys are redacted in preview and
doctor output but are still saved to the local `.env.dev` files with `0600`
permissions.
