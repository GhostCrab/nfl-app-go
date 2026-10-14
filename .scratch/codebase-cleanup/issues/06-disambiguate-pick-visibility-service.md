# 06: Disambiguate the duplicated PickVisibilityService name

**What to build:** Reading or writing visibility code no longer requires checking which import is in scope to know what a type does.

Two different types share the name `PickVisibilityService`. One lives in the models layer and evaluates the visibility rules themselves (constructed with no arguments, exposes rule calculation and debug-time control). The other lives in the services layer, wraps the first, and is what handlers actually call to filter picks (constructed with a game service). Their constructors have the same name and different signatures, and their filter-facing methods differ in both shape and argument order.

The rules type is referenced in only two places, both inside the services wrapper, so renaming it to say what it is — rule evaluation, not a service — is contained. Rename the models-layer type and its constructor; the wrapper keeps its current name since that is the one handlers depend on.

This is a clarity fix, not a defect. Nothing is broken; the names simply invite a wrong call that compiles.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] The models-layer type and its constructor are renamed to describe rule evaluation
- [ ] The services wrapper keeps its name and public method signatures unchanged
- [ ] No handler or route behaviour changes
- [ ] `CLAUDE.md`'s gotcha note about the duplicated name is updated or removed
