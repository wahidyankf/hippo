# Worktree Location

Task worktrees for this repository live in `hippo-worktrees/` **beside** the checkout, never inside it. The sibling repository puts them under `worktrees/` at its own root. This one may not, and the reason is the Go toolchain rather than taste.

## The Rule

- Create every task worktree at `../hippo-worktrees/<name>/`, one per plan or task, reused for every delivery unit that work produces.
- Never create a directory named `worktrees/` inside this repository, and never add one to `.gitignore` — an ignore entry invites the layout this document refuses.
- Everything else about worktrees follows the [integration path](integration-path.md): initialize with `npm ci` before any gate run, sync by rebase, delete the worktree, the local branch, and the remote branch once every unit has landed.

## Why

`go build` resolves the version-control root by walking **up** from the module and taking the outermost directory holding a `.git`. It does not accept a `.git` _file_, which is what a linked worktree has — so it walks straight past the worktree and keeps going.

What the next `.git` up turns out to be depends on the clone, and neither answer is safe. Bareness is a per-clone property rather than a fact about this repository: verify it with `git worktree list`, reading the `(bare)` marker, and never with `git rev-parse --is-bare-repository`, which answers the narrower "is _this checkout_ bare" and returns `false` from inside a linked worktree by design.

Where the clone is bare, `git status` is fatal by definition and the build stops loudly:

```
error obtaining VCS status: exit status 128
	Use -buildvcs=false to disable VCS stamping.
```

Where the clone has a primary checkout, the failure is quieter and worse. With the worktree inside, `go build` finds that checkout's `.git`, succeeds, and stamps the binary with **its** revision plus `vcs.modified=true` — provenance belonging to a checkout that contributed nothing to the build, and nothing anywhere says so. The rule holds whichever shape a clone has, which is the point: nobody has to check the topology before obeying it.

Placed beside the checkout, there is no `.git` directory above the worktree at all. Go stamps nothing rather than stamping a lie, and `scripts/build-release.sh` — which clones into a temporary directory that does have a real `.git` — keeps its own stamping intact.

## What This Does Not Change

`-buildvcs=false` is not the alternative. It would silence the error while leaving every other consumer of the walk to find the same wrong root, and it would disable stamping for builds that are entitled to it.

The sibling repository's containment rule is correct there and is not a rule this repository failed to adopt. Cargo does not walk out of a worktree looking for a version-control root; `go build` does. See [rules propagation](../workflows/rules-propagation.md) for why a rule that fits one repository is not thereby owed to another.
