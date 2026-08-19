
# Refactor

- zimfs/Queue.go: EnqueueURL() needs to be simpler, html body is used for Detect() but might be better without this parameter
- zimfs/QueueEntry.go: filterQueueEntryURL() is a little unnecessary, maybe can be moved to Enqueue() directly

## internal/utils/zim

- Move Render methods to `internal/utils/zim` namespace

## internal/utils/zimfs

- Move Searcher to zimfs namespace, use NewSearcher(folder string) and load files the same way that Manager does


## internal/filters

- Implement "PHPBB" (in PHPBB.go) filter that maps threads and forum links to better URLs, similar to how MediaWiki rewrites the URLs.
  But map URLs more nicely like so:
- `https://pckf.com/viewforum.php?f=1` should be `pckf.com/forum/1.html`
- `https://pckf.com/viewtopic.php?t=21&sid=d23dfa4ecebe258c3f46724462bd0d59` should be `pckf.com/topic/21.html`

- Filter out memberlist.php URLs because they're behind a login wall anyways
  `https://pckf.com/memberlist.php?mode=viewprofile&u=18671&sid=d23dfa4ecebe258c3f46724462bd0d59`

# Web UI

- On the Archive Page add a button that Detects the Settings/Filters automatically and selects them.
  The idea is to enter the StartURL, then click "Auto-Detect" or similar and then the correct filters will be selected automatically.
  Makes it easier for new users to decide whether to use MediaWiki or PHPBB or no filter etc.
  By default, Trackers and Scripts filter should be activated in all cases.

