# Learning from past reviews

Every submitted review is kept, which makes the reviewer's habits readable
rather than guessed at:

```
sbnn reviews --stats                     # which files draw comments, how many per review
sbnn reviews --comments --since 30d      # one line per comment, to read properly
```

For any question `--stats` does not answer, `--comments` emits one record
per comment and ordinary tools do the counting. Parse the jsonl form — one
flat JSON object per line, and the only form that carries whole comment
bodies; the tab-separated text form (date, group, path:lines, author,
first body line) is for reading and quick pipes:

```
sbnn reviews --comments --format jsonl | jq -r 'select(.suggestions) | .path'
sbnn reviews --comments | cut -f3 | cut -d: -f1 | sort | uniq -c | sort -rn
```

Worth doing before you hand over a change of the same shape: if the last ten
reviews of this repository were mostly about error messages and test names,
check yours before asking. Say what you found and what you changed because of
it — a pattern you read out of the log is a claim about the human, so let
them correct it.
