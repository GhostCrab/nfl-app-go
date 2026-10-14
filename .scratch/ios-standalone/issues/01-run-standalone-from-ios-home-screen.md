# 01: Run as a standalone app from the iOS home screen

**What to build:** Adding the site to an iPhone home screen launches it as its own app — no Safari chrome, its own icon, content correctly inset from the status bar, notch and home indicator on every page.

**Blocked by:** None (can start immediately)

**Status:** resolved

## Audit

Asked to "audit `viewport-fit=cover` and address it", the audit found the gap was wider than the one attribute.

**Nothing supported standalone launch.** No web app manifest, no `apple-mobile-web-app-capable`, no `apple-touch-icon`. Add to Home Screen produced a Safari bookmark, not an app, and would have shown a screenshot where the icon belongs.

**`viewport-fit=cover` was absent from all seven pages**, which is what made every `env(safe-area-inset-*)` in the stylesheet evaluate to `0px`. There is no base layout template, so the `<head>` is duplicated seven times — one logical change meant seven edits.

**The fixed toolbar had no top inset.** `.mat-toolbar` is pinned to `top: 0` with a fixed height. Opting into `viewport-fit=cover` without addressing it would have slid the navigation under the status bar — the change would have *caused* a bug rather than fixed one.

**The toolbar's height was hardcoded in three places** with no shared definition: the bar itself, the dashboard container's offset, and a second copy inlined in the odds details page. Growing the bar by the status bar inset would have desynchronised all three.

**A pre-existing bug, unrelated to iOS:** the analytics page's container had *no* toolbar offset at all, so its first rows have always been hidden behind the fixed bar on every browser and platform. Worth knowing this was not introduced here.

**Nothing anywhere accounted for the bottom or sides.** In standalone mode the web view owns the full screen, so content runs under the home indicator, and under the notch in landscape. The theme toggle on the three auth pages is absolutely positioned against the viewport at `top: 1rem`, which lands beneath the status bar.

## What was done

A shared head partial now declares the viewport and install metadata once for all seven pages. A manifest and three generated PNG icons (180/192/512, redrawn from the existing favicon geometry) make the app installable. The toolbar measurement and the four insets became custom properties on `:root`, so the bar and all three page containers derive from one definition. The toolbar grows into the status bar area and tints it rather than hiding beneath it; `body` carries the bottom and side insets; the analytics container gained the offset it never had.

The server now registers the `.webmanifest` MIME type, because Go has no built-in mapping for it and Safari rejects a manifest served as sniffed `text/plain` — which would have broken standalone launch silently.

## Decisions taken

- **`black-translucent` status bar.** The app defaults to a dark theme, so an opaque light status bar would clash. This draws the indigo toolbar behind the status bar, which is why the toolbar needed the top inset.
- **No service worker.** Standalone launch on iOS does not require one, and offline caching is actively wrong for an app whose entire purpose is live scores.
- **No orientation lock.** Deliberately omitted from the manifest; locking rotation is a product decision, and the side insets were implemented precisely so landscape works.

- [x] Add to Home Screen installs with the app icon rather than a screenshot
- [x] Every page opts into `viewport-fit=cover` from a single declaration
- [x] The toolbar measurement has one definition that all consumers derive from
- [x] The fixed toolbar clears the status bar and tints it
- [x] Content clears the home indicator and the landscape notch
- [x] The auth pages' theme toggle clears the status bar
- [x] The analytics page's content clears the toolbar (pre-existing bug)
- [x] The manifest is served with a MIME type Safari accepts

### Needs a real device

- [ ] Launches chrome-free from the home screen after Add to Home Screen
- [ ] Status bar renders legibly over the toolbar, both themes
- [ ] No content sits under the home indicator, portrait or landscape
- [ ] The home screen icon renders correctly at the OS mask
