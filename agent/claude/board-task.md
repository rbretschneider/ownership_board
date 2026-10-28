---
description: File what we just agreed in this chat as an autonomous ownership_board ticket that runs to a pull request with no stops.
argument-hint: "[optional: what to file, if not the last thing agreed]"
---

The owner and you have agreed a piece of work in this chat. File it on the
ownership_board as an **autonomous** ticket: the board's runner takes it from
Ready through design, build, QA, security and GPT review to a pull request,
without checkpoints or questions. The owner's only review is the pull
request, so the spec you write is all the agents will have.

`$ARGUMENTS`, if given, says which part of the chat to file.

## 1. Pin down what was agreed

Use only what the owner actually said or accepted in this chat. If the request,
its scope or its repo is not settled, say what is open and stop; do not file
a guess. One ticket per independent piece of work: if the chat agreed two,
file them one at a time.

## 2. Pick the repo

Call `list_repos` (the `ownership` MCP server) and find the repo the chat was
about. If it is not listed or more than one fits, ask the owner which and stop.

## 3. Write the spec

Markdown, in the shape the board's refinement produces:

- **Problem**: what is wrong or missing, and for whom.
- **Scope** and **Out of scope**.
- **Acceptance criteria**: a numbered list, each one testable.
- **Decisions agreed in chat**: every choice the owner made or accepted,
  with the reason, so agents treat them as settled.
- **Approach and risk**: what you learned about the code in this chat (files,
  patterns, pitfalls), and what could go wrong.
- A last line `Build tier: small | normal | large` and one sentence on why:
  small for copy, config or a one-file fix; normal for a few files; large for
  cross-cutting, security- or data-sensitive, schema or auth changes. Pick the
  larger when unsure.

Never put secrets, tokens, passwords or private keys in the spec. Keep it
under 20,000 characters, with no raw HTML or HTML comments.

## 4. Show it and get a yes

Show the owner the title, kind (story or bug), repo and the whole spec, then
ask: "File this as an autonomous ticket?" Wait for an explicit yes to this
text. A yes to something earlier in the chat does not count, and neither does
silence. If the owner changes anything, show it again.

## 5. File it

Call `create_autonomous_ticket` with `kind`, `title`, `body` (the request in
two or three sentences, in the owner's words), `spec` and `repo_id`.

- On success, report the ticket number and that the runner will pick it up on
  its next poll. Do not build it here, and do not claim it.
- If the board refuses because the token is not a delegate, tell the owner:
  the desktop's `ownership` MCP server needs a delegate token (Settings, Agent
  tokens, "Delegate"). Do not fall back to `create_ticket`.
