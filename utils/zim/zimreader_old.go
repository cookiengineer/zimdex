package zim

// list all title pointer, Titles by position contained in a zim file
// Titles are pointers to URLpos index, usefull for indexing cause smaller to store: uint32
// note that this is a slow implementation, a real iterator is faster
// you are not suppose to use this method on big zim files prefer ListTitlesPtrIterator to build your index
func (z *ZimReader) ListTitlesPtr() <-chan uint32 {
	ch := make(chan uint32, 10)

	go func() {
		var pos uint64
		var count uint32

		for pos = z.titlePtrPos; count < z.ArticleCount; pos += 4 {
			idx, err := readInt32(z.bytesRangeAt(pos, pos+4))
			if err != nil {
				continue
			}
			ch <- idx
			count++
		}
		close(ch)
	}()
	return ch
}

// list all title pointer, Titles by position contained in a zim file
// Titles are pointers to URLpos index, usefull for indexing cause smaller to store: uint32
func (z *ZimReader) ListTitlesPtrIterator(cb func(uint32)) {
	var count uint32
	for pos := z.titlePtrPos; count < z.ArticleCount; pos += 4 {
		idx, err := readInt32(z.bytesRangeAt(pos, pos+4))
		if err != nil {
			continue
		}
		cb(idx)
		count++
	}
}

// return the article at the exact url not using any index
func (z *ZimReader) GetPageNoIndex(url string) (*Article, error) {
	// starting at 1 to avoid "con" entry
	var start uint32
	stop := z.ArticleCount

	a := new(Article)

	for {
		pos := (start + stop) / 2

		offset, err := z.OffsetAtURLIdx(pos)
		if err != nil {
			return nil, err
		}
		err = z.FillArticleAt(a, offset)
		if err != nil {
			return nil, err
		}

		if a.FullURL() == url {
			return a, nil
		}

		if a.FullURL() > url {
			stop = pos
		} else {
			start = pos
		}
		if stop-start == 1 {
			break
		}

	}
	return nil, errors.New("article not found")
}

// get the offset pointing to Article at pos in the URL idx
func (z *ZimReader) OffsetAtURLIdx(idx uint32) (uint64, error) {
	offset := z.urlPtrPos + uint64(idx)*8
	return readInt64(z.bytesRangeAt(offset, offset+8))
}

// populate the ZimReader structs with headers
// return start and end offsets for cluster at index idx
func (z *ZimReader) clusterOffsetsAtIdx(idx uint32) (start, end uint64, err error) {
	offset := z.clusterPtrPos + (uint64(idx) * 8)
	start, err = readInt64(z.bytesRangeAt(offset, offset+8))
	if err != nil {
		return
	}
	offset = z.clusterPtrPos + (uint64(idx+1) * 8)
	end, err = readInt64(z.bytesRangeAt(offset, offset+8))
	end--
	return
}
