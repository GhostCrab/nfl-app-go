# 05: Make building the whole module the documented command

**What to build:** Building and vetting the entire module succeeds, and `CLAUDE.md` tells agents to do exactly that instead of warning them away from it.

Right now `CLAUDE.md` carries a prominent caveat that a whole-module build fails for reasons unrelated to any change, and instructs agents to scope builds to the root package plus whatever they touched. That guidance is correct today and becomes actively misleading the moment tickets 02 and 03 land — an agent told to ignore build failures will ignore real ones.

Replace the caveat with the now-true commands. Separately, the pre-existing vet warnings (unkeyed BSON element literals, and address formatting that does not handle IPv6 in the email service) should either be fixed or explicitly acknowledged as accepted, so "vet is clean" means something.

Also drop the stale note about the interface package's parameter order disagreeing with the real visibility filter, which stops being a hazard once that package is gone.

**Blocked by:** 02, 03

**Status:** ready-for-agent

- [ ] A whole-module build succeeds from a clean checkout
- [ ] A whole-module vet is either clean or its remaining findings are documented as accepted
- [ ] `CLAUDE.md`'s Commands section recommends the whole-module build
- [ ] The "fails repo-wide" caveat and the interface parameter-order note are removed from `CLAUDE.md`
