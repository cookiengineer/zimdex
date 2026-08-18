
# Refactor

- zimfs/Queue.go: EnqueueURL() needs to be simpler, html body is used for Detect() but might be better without this parameter
- zimfs/QueueEntry.go: filterQueueEntryURL() is a little unnecessary, maybe can be moved to Enqueue() directly
