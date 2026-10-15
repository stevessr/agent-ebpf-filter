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


## Desktop apps: Claude Desktop and ChatGPT Desktop

Desktop apps are distinct from the similarly named command-line harnesses.
Renew does not assume that a CLI in PATH is the desktop application.

- Windows: Query per-user AppX/MSIX registrations with PowerShell for the
  precise package families Claude_pzs8sxrjxfjjc,
  OpenAI.ChatGPT-Desktop_2p2nqsd0c76g0, and
  OpenAI.Codex_2p2nqsd0c76g0. The latter two are separate ChatGPT Store
  generations; if both are present, Renew refuses to select one automatically.
- ChatGPT: Default to Microsoft Store update management. Only if an exact
  package/Store ID shows up in a source-filtered WinGet installed list can
  Renew offer an opt-in single-app upgrade using the corresponding ID
  (9NT1R1C2HH7J or 9PLM9XGG6VKS). Newer package IDs may change; registration
  is the authority, not the application process or executable name.
- Claude: The official Windows installer uses MSIX and can be governed by
  Claude auto-update or enterprise MDM. When the exact Anthropic.Claude ID is
  also present in WinGet's installed list, show a clearly labeled
  "winget-compatible" manual update option, not a claim that WinGet originally
  installed the app. Do not mix this with MDM or self-managed updates.
- macOS: Detect /Applications and user ~/Applications bundles. Because a
  bundle path alone does not establish ownership of Homebrew, MAS or a vendor
  updater, show in-app/original-source guidance only.
- Linux: Claude Desktop from the official APT repo is recognized through
  its claude-desktop binary and dpkg ownership. Unknown/nonofficial
  ChatGPT desktop variants receive read-only guidance.

## Windows CLI package managers

- Scoop: Check the current command under Scoop apps/shims, exact package
  prefix, installed list entry, and (for shims) the result of scoop which.
  Update by explicitly invoking the trusted Scoop command in a fixed
  PowerShell script with a validated package token.
- Chocolatey: Require a binary under its bin shim directory and an exact
  installed package entry. Updates use choco upgrade PACKAGE and no flags
  that force reinstall all packages.
- WinGet: Use exact package IDs and explicit winget/msstore source flags.
  WinGet's matching catalogue does not prove which installer originally
  put an app on the machine: its result is marked "compatible" unless an
  independent package registration proves the intended app identity.

Read-only discovery never downloads or executes new code. Updates may require
network access or user elevation, and an enterprise package manager can have
a different update policy from a personal machine.

Official reference sources:
- https://support.claude.com/en/articles/12622703-deploy-claude-desktop-for-windows
- https://support.claude.com/en/articles/10065433-install-claude-desktop
- https://help.openai.com/en/articles/9982051-using-the-chatgpt-windows-app
- https://learn.microsoft.com/en-us/windows/package-manager/winget/upgrade
- https://github.com/ScoopInstaller/Scoop/wiki/Commands
- https://docs.chocolatey.org/en-us/choco/commands/upgrade/
