
# Refactor

- zimfs/Queue.go: EnqueueURL() needs to be simpler, html body is used for Detect() but might be better without this parameter
- zimfs/QueueEntry.go: filterQueueEntryURL() is a little unnecessary, maybe can be moved to Enqueue() directly

## internal/utils/zim

- Move Render methods to `internal/utils/zim` namespace

## internal/utils/zimfs

- Move Searcher to zimfs namespace, use NewSearcher(folder string) and load files the same way that Manager does


# internal/archive

- Move internal/zimfs to internal/io/zimfs
- Move internal/zim to internal/archive/zim

