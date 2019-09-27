package autoscaler

import "container/ring"

func isBufferFilled(buffer *ring.Ring) bool {
	var isFilled = true
	buffer.Do(func(value interface{}) {
		if value == nil {
			isFilled = false
			return
		}
	})
	return isFilled
}

func bufferFulfillmentDegree(buffer *ring.Ring) int {
	var result int = 0
	buffer.Do(func(value interface{}) {
		if value != nil {
			result++
		}
	})
	return result
}

func clearBuffer(buffer *ring.Ring) {
	b := buffer
	if b != nil {
		for i := 0; i < b.Len(); i++ {
			b.Value = nil
			b = b.Next()
		}
	}
}
