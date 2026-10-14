# 01: Submit Picks button unreachable on iOS browsers with tall chrome

**What to build:** A user opening the pick picker on a phone can always reach and press Submit Picks, whatever browser they use and however tall its toolbars are. The games list scrolls; the footer holding Submit and Cancel stays pinned and visible inside the dialog.

**Reported:** Brave on iOS. The Submit Picks button sits below the bottom of the screen, hidden behind Brave's bottom toolbar, and the user cannot scroll to it — scrolling moves the games list but never brings the button into view.

## Diagnosis

Two independent defects combine here. Either alone is survivable; together they produce an unreachable button. This is a strong hypothesis from reading the layout, not a device-confirmed finding — see verification below.

**1. The dialog's flex chain is broken, so the footer is clipped rather than pinned.**

The modal is a flex column that hides its overflow, and its direct children are the header and the form. The scrolling content area and the footer are children of *that form*, which is not itself a flex container and is not told to fill or shrink. So the `flex` and `min-height` properties on the content area are inert — they describe a relationship to a parent that isn't a flex container.

The practical result: the content area grows to its own hardcoded maximum, the footer is laid out after it, and the footer falls past the modal's hidden-overflow boundary. It is clipped, not merely off-screen, and the only scrollable element is the games list — which scrolls its own contents and can never reveal the footer. That is precisely the reported "can't scroll to it."

**2. Viewport height is overestimated on iOS, and the slack absorbing that error is a hardcoded guess.**

Three places size the dialog by subtracting the safe-area insets and a magic number (currently 40px for the dialog, 160px for the content area, 20px on small screens) from `100vh`. On iOS, `100vh` resolves to the *large* viewport: it does not shrink to account for visible browser chrome. The safe-area insets cover the home indicator and notch, not the browser's own toolbars. So the computed height overshoots the genuinely usable area by roughly the toolbar height.

Shorter chrome keeps that overshoot inside the hardcoded slack, which is why this looks fine in most browsers. Brave's bottom toolbar on iOS is taller than Safari's, the overshoot exceeds the slack, and the dialog's bottom edge — where defect 1 has already stranded the footer — lands underneath the toolbar.

**Do not special-case Brave.** Every iOS browser renders in WebKit, so this is iOS viewport semantics plus taller chrome, not a Brave engine quirk. Any browser or OS version with taller toolbars reproduces it, and user-agent sniffing would leave those broken while adding a branch to maintain.

## Direction

Fix the flex chain first, then the measurement: with a correct chain the footer can no longer escape the dialog regardless of height, which makes the height fix a simple substitution rather than a new round of magic numbers. Make the form participate in the column layout so the content area genuinely absorbs the free space and the footer is always laid out inside the dialog. That should let all three hardcoded content-area maximums be deleted outright rather than retuned.

For the measurement, prefer dynamic viewport units, which track browser chrome as it appears and retract and are available on the iOS versions in question, with the current units kept as a fallback for older browsers. Keep the safe-area insets; they solve a real and different problem.

Solve this in CSS. The project's standing constraint is minimal JavaScript, and a layout that depends on a resize listener will be more fragile than one the browser maintains itself.

**Blocked by:** None (can start immediately)

**Status:** resolved

### Agent-verifiable

- [x] The footer is a laid-out part of the dialog at any viewport height, not clipped by overflow
- [x] The games list is the only scrolling region; it scrolls independently of the footer
- [x] No hardcoded pixel slack remains in the dialog's height calculation
- [x] Dialog height responds to browser chrome appearing and retracting, with a fallback for browsers lacking dynamic viewport units
- [x] Safe-area insets still apply, so the footer clears the home indicator — now genuinely satisfied: viewport-fit=cover landed with the iOS standalone work.
- [x] No user-agent or browser-brand detection is introduced
- [x] Submit and Cancel remain reachable at the narrow breakpoint and in landscape
- [x] Submitting picks still posts and swaps its result as before

### Needs the reporter or a real device

- [x] Confirmed fixed on Brave for iOS by the user who reported it
- [x] Spot-checked on Safari for iOS to confirm no regression where it already worked — accepted unverified at the owner’s direction.

## Comments

**Implementation finding — the safe-area criterion cannot be met by CSS alone.**

`env(safe-area-inset-*)` only resolves to a non-zero value when a page opts its viewport into the unsafe area with `viewport-fit=cover`. No template in this repo does; every one is `width=device-width, initial-scale=1.0`. So every safe-area inset here — including the three that predate this work — has always evaluated to `0px`.

This means the criterion "Safe-area insets still apply, so the footer clears the home indicator" is **not satisfied by the stylesheet**. The footer clears the home indicator anyway, because without `viewport-fit=cover` the browser insets the viewport itself and the dialog never extends under the indicator. The mechanism differs from what the criterion assumed; the user-visible outcome is correct.

The inset expressions were kept, now with explicit `0px` fallbacks, so they begin working the moment `viewport-fit=cover` is added. Adding it was deliberately left out of scope: it opts the entire application into drawing under the notch and home indicator, so the nav bar, dashboard, and every edge-anchored element would need safe-area padding audited on a real device. That is its own ticket, not a rider on this fix.

This does not gate the reported bug. Submit Picks being unreachable was caused by the broken flex chain and the large-viewport measurement, both of which are fixed here and neither of which depends on the insets.

**Also corrected during review:** the first version of this fix used `100vh` as the pre-`dvh` fallback. On iOS below 15.4 that is strictly worse than the `height: 100%` it replaced, since `100vh` is the large viewport while `100%` on a fixed overlay tracks the visual viewport. The fallback is now `height: 100%`.

**Resolved 2026-10-13.** The reporter confirmed the Submit Picks button is reachable on Brave for iOS. Fix shipped in commit 433e1a0.

Two criteria remain unticked and neither blocks this ticket:

- The safe-area inset criterion cannot be satisfied by the stylesheet, for the reason recorded above. The footer clears the home indicator by a different mechanism, so the user-visible outcome is correct. Enabling `viewport-fit=cover` is a separate, application-wide change.
- The Safari for iOS spot-check was never run. The risk it covers is a regression on a configuration that already worked, which is low here: the change removed height ceilings rather than adding them, so a dialog that previously fitted still fits.

**Follow-up closed 2026-10-13.** `viewport-fit=cover` has since been added across every page as part of the iOS standalone work, so the safe-area insets in this dialog are live rather than inert. The criterion above that could not be met in CSS alone is now met. The Safari spot-check was waived by the owner.
