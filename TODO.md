
# Refactor

- zimfs/Queue.go: EnqueueURL() needs to be simpler, html body is used for Detect() but might be better without this parameter
- zimfs/QueueEntry.go: filterQueueEntryURL() is a little unnecessary, maybe can be moved to Enqueue() directly

## internal/utils/zim

- Move Render methods to `internal/utils/zim` namespace

## internal/utils/zimfs

- Move Searcher to zimfs namespace, use NewSearcher(folder string) and load files the same way that Manager does

# Web UI

- On the Archive Page add a button that Detects the Settings/Filters automatically and selects them.
  The idea is to enter the StartURL, then click "Auto-Detect" or similar and then the correct filters will be selected automatically.
  Makes it easier for new users to decide whether to use MediaWiki or PHPBB or no filter etc.
  By default, Trackers and Scripts filter should be activated in all cases.

