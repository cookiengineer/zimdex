package zim

import "github.com/hashicorp/golang-lru"
import "sync"

var article_pool sync.Pool
var bcache *lru.ARCCache

func init() {

	article_pool = sync.Pool{
		New: func() interface{} {
			return new(Article)
		},
	}

	// keep 15 latest uncompressed blobs, around 1M per blob
	bcache, _ = lru.NewARC(16)

}
