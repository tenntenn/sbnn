import os
import sys
# usage: gen.py round nfiles nmd fixture_dir -> unified diff on stdout; the
# Markdown files are also written under fixture_dir so previews read the tree.
# The needle zz&#109;dneedle400 is in docs/r20/doc1.md as an HTML entity: the
# diff carries the source text, the rendered preview carries "zzmdneedle400".
r = int(sys.argv[1]); n = int(sys.argv[2]); nmd = int(sys.argv[3]); fx = sys.argv[4]
H = "dif" + "f --" + "gi" + "t"
def newfile(path, lines):
    out = [f"{H} a/{path} b/{path}", "new file mode 100644", "index 0000000..1111111",
           "--- /dev/null", f"+++ b/{path}", f"@@ -0,0 +1,{len(lines)} @@"]
    out += ["+" + l for l in lines]
    return "\n".join(out) + "\n"
def modfile(path, lines):
    old = lines
    out = [f"{H} a/{path} b/{path}", "index 1111111..2222222 100644", f"--- a/{path}", f"+++ b/{path}",
           f"@@ -1,{len(old)} +1,{len(old)} @@"]
    for i, l in enumerate(old):
        if i == len(old) // 2:
            out.append("-" + l); out.append("+" + l + " // changed")
        else:
            out.append(" " + l)
    return "\n".join(out) + "\n"
s = ""
for i in range(n):
    if i < nmd:
        p = f"docs/r{r}/doc{i}.md"
        lines = []
        for k in range(300):
            if k % 25 == 0: lines.append(f"## Section {k//25}")
            elif k % 25 == 12: lines.append("```go")
            elif k % 25 == 13: lines.append("func f() { println(1) }")
            elif k % 25 == 14: lines.append("```")
            else: lines.append(f"Paragraph {k} of document {i} round {r} with some **bold** text and `code` and a [link](doc{i}.md).")
        if r == 20 and i == 1: lines[7] = "Only in the preview: zz&#109;dneedle400 comes from an entity."
        s += newfile(p, lines)
        full = os.path.join(fx, p)
        os.makedirs(os.path.dirname(full), exist_ok=True)
        with open(full, "w") as f:
            f.write("\n".join(lines) + "\n")
    else:
        p = f"pkg{i%10}/r{r}_file{i}.go"
        lines = ["package p", ""] + [f"func F{k}() int {{ return {k}*{i} }}" for k in range(38)]
        s += modfile(p, lines) if i % 2 else newfile(p, lines)
sys.stdout.write(s)
