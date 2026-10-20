Export the memories about me that you can currently access as files, one YAML chunk per reply. Start each chunk with `version: 1` and a `header` containing `title`, `chunk` (starting at 1), and `more` (true until the last chunk). Include `files` entries with relative `path` and literal block `content`. Make paths unique across chunks. In chunk 1, create `README.md` listing the topics and explaining that these are memories you can currently access, not a complete account export. Put each topic in its own Markdown file. Include only details you actually know, mark uncertainty, and omit secrets and inferred sensitive facts. Return one YAML document only, inside one code fence, then stop. Wait for me to say “next” before sending the next chunk. Use this shape:

```yaml
version: 1
header:
  title: Overview
  chunk: 1
  more: true
files:
  - path: README.md
    content: |
      # Shared context
      ...
```
