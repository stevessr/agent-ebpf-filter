# Renew: Coding Harness updates

The terminal page hosts a **Harness 更新管理** panel next to its launcher.
This feature runs in the desktop user's process. It does not contact the
privileged Agent eBPF collector, modify tracking rules, or auto-install tools.

## Workflow

1. Opening the terminal triggers a read-only installation-source scan.
2. Select a CLI and review its PATH entry, package name, source and argv.
3. Click **预览并准备更新** and explicitly **确认执行更新**.
4. Renew rechecks executable ownership just before executing the command
   in an ordinary-user PTY. Logs and exit status remain visible there.
5. Use **重新检测** after completion to refresh the recorded source.

No shell command interpolation, automatic remote installer scripts, privileged
backend API, background auto-update or silent sudo is used. System packages
may request sudo authorization interactively in the user's terminal.

## Supported channels

| Source | Evidence required | Update action |
| --- | --- | --- |
| pnpm | Exact package manifest under global root, or global shim + pnpm inventory + matching shim | pnpm add -g PACKAGE@latest |
| npm | Exact package manifest under npm global root | npm install -g PACKAGE@latest |
| Yarn Classic | Exact package manifest under Yarn global directory | yarn global add PACKAGE@latest |
| Bun | Exact package manifest under Bun global node_modules | bun add -g PACKAGE@latest |
| pacman | pacman owner query for PATH executable | sudo pacman -S --needed -- PACKAGE |
| APT | dpkg owner query matching path | sudo apt-get install --only-upgrade -- PACKAGE |
| DNF | rpm ownership query | sudo dnf upgrade -- PACKAGE (or dnf5) |
| Snap | Snap export path and confirmed snap package | sudo snap refresh PACKAGE |
| Flatpak | Flatpak export path and confirmed app ID | flatpak update APP_ID |
| Official script | Known Claude native version-managed installation path | existing claude executable with update argument |

The current executable's owner wins over the mere availability of managers.
For example, a CLI owned by pacman is not upgraded by npm just because npm
exists. Only known CLI IDs are mapped to Node package names. Unknown or
mismatched sources remain read-only, and project-local node_modules/.bin
installations are never turned into global ones.

Aider, Kiro and unrecognized official-script installations currently show
manual guidance instead of an unsafe guess. Yarn Berry/PnP, portable binaries,
aliases, custom Bun global directories and exports not reachable via the
tool's PATH name may also require manual updating. The existence of a vendor
installer does not prove ownership of a particular local executable.

Metadata probes use a timeout and bounded output. The scanner performs no
network requests itself. A confirmed package-manager update may use the
manager's ordinary network download process.
