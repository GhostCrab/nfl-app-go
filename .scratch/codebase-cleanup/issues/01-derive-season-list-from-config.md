# 01: Derive the season list from CURRENT_SEASON

**What to build:** Rolling the app to a new NFL season requires editing only `CURRENT_SEASON` in `.env`. Today four hardcoded season arrays must also be extended by hand, and a missed one silently drops the new season from legacy imports, club-score initialization, and stats queries.

One shared helper produces "all seasons with data" (earliest season through the configured current season), and every consumer uses it:

- `LegacyImportService.ImportAllLegacyData` and `GetImportSummary`
- `MemoryParlayScorer.InitializeClubScores` — this one is a live bug, not just a rollover hazard: it already accepts `currentSeason` as a parameter and then ignores it in favour of the hardcoded array, so club scores never initialize for a season newer than the literal list.
- `PickService.GetPickStats` — note this call site's loop body is a placeholder that writes `"Season N data"` strings rather than running real aggregation. Either point it at the derived range or remove the fake stats; don't preserve invented output.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] A single helper is the only place that knows the earliest season with data
- [ ] No `seasons := []int{...}` literal remains anywhere in the codebase
- [ ] `InitializeClubScores` honours the season it is passed
- [ ] A test proves a newly configured `CURRENT_SEASON` appears in the derived range with no Go source edits
- [ ] `GetPickStats` either reports real per-season figures or stops reporting placeholder strings
- [ ] README's "Annual Season Updates" section no longer lists season arrays as manual steps
