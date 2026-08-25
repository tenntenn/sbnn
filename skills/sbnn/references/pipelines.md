# Fitting sbnn into what you were already doing

sbnn is one command among the ones you already run, so let the shell do the
joining rather than looking for a flag:

```
git diff | sbnn                          # anything that writes a diff feeds it
sbnn comments | pbcopy                   # anything that reads text takes it
sbnn reviews --format jsonl | jq ...     # a line per review, for whatever asks
```

`sbnn comments` and `sbnn wait` say what they found in their exit status too — 0
when there is nothing to address, 1 when there is, and 2 from `sbnn wait` when
the review has not happened yet — so a review can gate what comes next
without anyone reading the output:

```
git diff | sbnn --target <topic>
sbnn wait --target <topic> -q && git commit -m "<message>"
```

That is the recommended way round committing, and it is worth being plain
with the user about why: send what you are about to commit, wait for the
review, and commit only once it comes back with nothing to address. sbnn has
no idea what a commit is and stays out of the way of one — it writes nothing
into the working tree, so `git status` says exactly what it said before you
started. When a review does have comments, address them and send the next
round before committing rather than committing over them.

If the change is already committed, review it the same way: `git show | sbnn`,
or `git diff <base>..HEAD | sbnn`. Use `--title` to say which is which, since
sbnn only sees the text.
