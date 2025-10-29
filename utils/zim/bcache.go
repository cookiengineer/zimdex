package zim

import lru "github.com/hashicorp/golang-lru"

var bcache *lru.ARCCache

func init() {

	// keep 15 latest uncompressed blobs, around 1M per blob
	bcache, _ = lru.NewARC(16)

}
