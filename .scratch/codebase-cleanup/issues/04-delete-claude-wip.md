# 04: Delete claude-wip.md

**What to build:** One source of truth for project constraints. `claude-wip.md` held the working agreements for agents (HTMX documentation as the reference, minimal JavaScript, the owner runs the app and owns the port, search the codebase unrestricted). Those now live in `CLAUDE.md`, which is loaded automatically every session, whereas `claude-wip.md` was only read when someone remembered to point at it.

Confirm nothing in it is missing from `CLAUDE.md` before removing it.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Every constraint in the file is present in `CLAUDE.md` or deliberately dropped
- [ ] `claude-wip.md` is deleted
