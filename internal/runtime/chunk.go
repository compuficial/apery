package runtime

type chunk struct {
	Start int64
	End   int64
	Index int
}

func makeChunks(total int64, size int64) []chunk {
	if total <= 0 {
		return nil
	}
	if size <= 0 {
		size = defaultChunkSize
	}

	capacity := int((total + size - 1) / size)
	chunks := make([]chunk, 0, capacity)
	index := 0
	for start := int64(0); start < total; start += size {
		end := start + size
		if end > total {
			end = total
		}
		chunks = append(chunks, chunk{Start: start, End: end, Index: index})
		index++
	}
	return chunks
}
