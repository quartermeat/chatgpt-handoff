# ChatGPT file handoff

This local tool asks ChatGPT for one YAML chunk at a time and imports each reply into a private archive. It never replaces an existing archive file.

Build and install with `make install`. This places `chatgpt-handoff` in `~/.local/bin`.

## Update flow

1. Run `chatgpt-handoff prompt` and paste the printed request into ChatGPT. `--copy` also copies it to the clipboard. The command saves a copy under `~/private/chatgpt-handoff/prompts/`. If no archive exists, it prints the first export request.
2. Save each ChatGPT reply to a file and run `chatgpt-handoff import reply.yaml`. You can also pipe a reply into `chatgpt-handoff import -`, or use `chatgpt-handoff import --paste`. The command saves the raw reply under `~/private/chatgpt-handoff/updates-raw/`, creates the archive files, and tells you whether to ask ChatGPT for the next chunk.
3. Run `chatgpt-handoff status` at any point to see the last date, imported chunk count, and next instruction.

`prompt`, `import`, and `status` accept `--archive DIR` for another archive location and `--json` for agent use. Archive and raw files are private (`0600`), and created directories use `0700`.

The original importer remains available as `chatgpt-handoff -input FILE -out DIR` for existing workflows. Run `make ci` to format-check, vet, and test the code.
