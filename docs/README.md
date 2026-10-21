# Documentation conventions

`docs/guides/` is the canonical copy of user documentation, readable on GitHub
and in a checkout. Aft currently has no documentation website and no embedded
`aft docs` command. Contributor material belongs in `docs/contributing/`.
Link to guides instead of duplicating them in the root README.

Each group is a folder with a `README.md`; each Markdown file in the group starts
with plain, single-line front matter:

```yaml
---
title: Workspaces
description: Register existing remote roots through SSH
order: 2
---
```

Keep a `# Title` heading for readers on GitHub. Group orders are unique; page
orders are unique within a group. Guide file stems are unique across groups.
Use relative links, including links outside the guides, and GitHub heading
fragments. `./x docs` checks this contract and local Markdown links across the
repository; it does not fetch external links or render a website. Use inline
Markdown links rather than reference-style links so the checker can verify them.

Start with [Start](guides/start/README.md) or [Usage](guides/usage/README.md).
