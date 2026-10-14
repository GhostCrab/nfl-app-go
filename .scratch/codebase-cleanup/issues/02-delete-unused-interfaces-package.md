# 02: Delete the unused interfaces package

**What to build:** The `nfl-app-go/interfaces` package compiles, by virtue of no longer existing. It currently does not compile at all: its compliance assertions (`var _ Iface = (*impl)(nil)`) have drifted from the real service signatures in roughly six places — missing methods, and parameter orders that disagree with the implementations.

Nothing in the codebase imports it. `main.go` wires concrete service types directly and handlers accept concrete `*services.X` values, so the package provides no abstraction seam — only a drift check that nothing ran. It drifted precisely because nothing depended on it.

Delete it rather than repairing it. If seams are wanted later for testing, declare the interface at the consumer that needs it, which is the Go idiom and keeps the contract next to its user.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The `interfaces` package is removed
- [ ] A repo-wide search confirms nothing referenced it
- [ ] The root binary still builds and the existing tests still pass
