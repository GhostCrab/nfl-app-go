# 03: One directory per CLI tool

**What to build:** Every standalone CLI tool builds on its own, and building the whole module no longer trips over them.

Eight single-file `package main` tools currently sit flat in two directories (`cmd/` and `scripts/`), several per directory. Go treats each directory as one package, so `main`, `getEnv`, and `getEnvWithDevelOverride` are redeclared and neither directory compiles. Give each tool its own package directory so it builds by path with no file-list juggling.

This is not a pure file move: `deploy.sh` builds two of these tools by explicit file path during deployment, and the README documents the same paths for manual backup and restore. Both must move in lockstep or production deployment breaks silently at the next deploy.

One tool (`import_legacy`) also passes the wrong number of arguments to its service constructor and will not compile even once isolated; fix the call while it is in front of you.

While reorganizing, flag any tool that is dead weight (there is at least one colour-output demo and one one-off odds admin tool) and propose deleting it rather than carrying it forward. Deleting is a separate decision — surface it, don't action it here.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Each tool lives in its own package directory and builds by path
- [ ] No duplicate `main` or helper declarations remain in either directory
- [ ] `import_legacy` compiles against the current service constructor signature
- [ ] `deploy.sh` builds the backup and restore tools from their new paths
- [ ] README's backup, restore, and script-building instructions match the new paths
- [ ] Candidates for deletion are listed in a comment on this ticket, not deleted
