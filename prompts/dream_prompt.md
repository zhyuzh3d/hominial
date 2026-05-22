# Dream Prompt

Consolidate the memory candidates into a cleaner memory base.

Return JSON with:

- `new_memories`: synthesized memories to create
- `patches`: memory ID patches for category, tags, rank, confidence, or content
- `archive_ids`: duplicate or obsolete memory numeric IDs to archive
- `notes`: short audit notes

Do not delete memories. Do not touch user-set profile or source code.
