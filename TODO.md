# goports: Roadmap & TODO

This document tracks planned improvements, ordered roughly by expected user impact
and implementation effort. Items marked **(high return)** should be tackled first.

## 1. High‑return enhancements

1. ✅ **Add UDP listeners and IPv6 support** – expand discovery to show UDP, IPv6
   and UNIX domain sockets. (cross‑platform groundwork)
2. ✅ **JSON output & scripting-friendly formats** – `--json`/`--csv` for CLI.
3. ✅ **Improve CLI filtering & search** – allow filtering by port, name, bundle,
   address family; implement `--signal` and kill-by-name options.
4. ✅ **Cache DNS/bundle lookups** – reduce cost of repeated reverse DNS and
   bundleID calls.
5. ✅ **Context/timeouts on external commands** – wrap `exec.Command` calls with
   contexts so slow `lsof`/`ps` don't hang.
6. **Cross-platform foundation** –
   * abstraction layer (`discoverPorts`) added so platform-specific
     implementations can replace `lsof` in future.
   * platform tagged mains created; non-darwin builds yield CLI-only binary
     avoiding GUI dependencies.
   * Next step: implement native enumeration on Linux/Windows.

## 2. GUI polish & usability

- ✅ Add search/filter box in menu-bar GUI.
- ✅ Dark mode support (icon variants) and basic accessibility support.
- ✅ Notifications per-port toggle with persisted settings.
- Preferences pane for refresh interval and protocols to show. (still
todo)

## 3. Developer experience & packaging

- Add unit tests for `ports.AppsByPort` (fake lsof output).
- Extract public library API and document it.
- Homebrew formula and GitHub Actions release workflow.
- Sign/notarize macOS bundle in CI.

## 4. Nice-to-have / long-term

- Local HTTP server for external tooling.
- Remote host support (SSH).
- iOS/watchOS companion widget.
- Real‑time port activity graphs.


> Each section can be broken into issues/PRs. Start by tackling high-return
> items; they’ll also lay groundwork for cross‑platform and testing work.