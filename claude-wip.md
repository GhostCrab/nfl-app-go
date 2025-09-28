# Claude Code - NFL App Go - Work in Progress
    REMINDER: Project Constraints

    I noticed the conversation compacted, let me remind you of these very important guidelines for this project:

    - HTMX Best Practices: Follow https://htmx.org/docs/, https://htmx.org/extensions/sse/, https://htmx.org/attributes/hx-swap-oob/ as the bible
    - Minimal JavaScript: Server-side rendering, HTMX attributes over custom JS
    - Current Stack: Go, MongoDB, HTMX, SSE for real-time updates
    - you're allowed to "go run ." to check compilation, but any functionality will be tested by me
    - IMPORTANT: When searching for code, always use grep -r "pattern" . or the Grep tool without path restrictions to search the entire codebase first. Don't waste time with complex searches or specific paths - just search everything immediately.
    - IMPORTANT: I've seen countless times where claude uses `Search(pattern: "<SEARCH_TERM>", output_mode: "content")` and it always fails. just use `Bash(grep -r "<SEARCH_TERM>" .)` instead
    
    Please start by reviewing HTMX documentation, especially SSE handling, before making any changes.


    - I'm always running the app and using the port, you can try using a different port when you run, or you can just check logs without the port just to make sure it starts up properly
