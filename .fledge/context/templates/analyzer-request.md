# Analyzer request instructions

Edit the prose inside the XML tags below. "fledge context compose
analyzer-request" copies each tag's contents into the request's
instructions_before and instructions_after fields, substituting {group_id},
{purpose}, and {worksheet_path} with the request's own values. Text outside
the tags is ignored.

<instructions_before>
You are a Fledge analyzer assigned file group "{group_id}". Group purpose:
{purpose}. This message is your task; act on it now and do not wait for any
further message. Read every file listed in "files" below by its exact
relative path, and only those files. Your worksheet is at {worksheet_path}:
fill it out as you work — it is your scratch pad and remains in the run
folder as your human-readable deliverable. Produce structured findings for
this group: subsystem summary, entry points, key symbols, internal and
external dependencies, data flows, invariants, tests, and one summary per
file.
</instructions_before>

<instructions_after>
When your analysis is complete:
1. Finish your worksheet at {worksheet_path}, then derive your structured
   reply from it. Save this request body unchanged to
   "request-{group_id}.json".
2. Write your completion as exactly one JSON object (no Markdown, no prose)
   to "reply-{group_id}.json", using the analyzer reply schema from your
   role instructions: "status":"ok" with the full analysis, or
   "status":"error" with at least one error entry.
3. Validate first:
   fledge context validate analyzer-reply --request request-{group_id}.json reply-{group_id}.json
4. Reply exactly once:
   fledge agent msg reply <message-id> --body-file reply-{group_id}.json
If validation rejects the file, correct it and retry. Never fall back to
"fledge agent msg send" and never send progress messages.
</instructions_after>
