# ChatGPT file handoff

This local tool turns each YAML chunk from ChatGPT into ordinary files. You can import the first chunk while ChatGPT prepares the next one. Review each chunk before importing it. Keep personal memory exports private. Existing files are never overwritten.

## Step 1: open the prompt in VS Code

The file you send to ChatGPT is [PROMPT.md](PROMPT.md). Open the project in VS Code with that file ready to copy:

```sh
code /home/quartermeat/work/chatgpt-handoff /home/quartermeat/work/chatgpt-handoff/PROMPT.md
```

In the editor, press Ctrl+A, then Ctrl+C, and paste the whole file into ChatGPT unchanged.

## Step 2: save and import each chunk

Paste the exact request in [PROMPT.md](PROMPT.md) into ChatGPT. It will return one chunk per reply, with a YAML header showing the topic, chunk number, and whether more chunks remain. Save each reply, without Markdown code fences, as `chunk-001.yaml`, `chunk-002.yaml`, and so on.

Import each chunk into the **same** folder as it arrives:

```sh
cd /home/quartermeat/work/chatgpt-handoff
go run . -input chunk-001.yaml -out /home/quartermeat/Documents/agent-handoff
go run . -input chunk-002.yaml -out /home/quartermeat/Documents/agent-handoff
```

Ask ChatGPT for the next chunk after you have saved the current one. The importer prints the header and every created path. The resulting folder can be copied or given to an agent directly. Each chunk uses `version: 1`, a `header`, and one or more `files` with relative `path` and text `content`. File paths must be unique across chunks. To start a fresh export later, choose a new output directory.
