
# Refactor

- zimfs/Queue.go: EnqueueURL() needs to be simpler, html body is used for Detect() but might be better without this parameter
- zimfs/QueueEntry.go: filterQueueEntryURL() is a little unnecessary, maybe can be moved to Enqueue() directly

## internal/utils/zim

- Move RenderHTML, RenderCSS, RewriteHTML, RewriteCSS methods to `internal/utils/zim` namespace
- Modify RewriteCSS to use a better filters.CSS
- Create an ApplyFilterCSS() method, so that plugins can also rewrite CSS

# internal/archive

- Move ./internal/zimfs to ./io/zimfs
- Move ./internal/zim to ./archive/zim

