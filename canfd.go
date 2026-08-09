package seg

import "iter"

var validFDCaps = []int{63, 47, 31, 23, 19, 15, 11, 7, 6, 5, 4, 3, 2, 1}

// CANFDStrategy yields discrete ISO 11898-1 CAN FD frame sizes with 0% padding.
func CANFDStrategy(segSize int) Strategy {
	validCaps := validFDCaps
	for i, v := range validCaps {
		if v+1 <= segSize {
			validCaps = validCaps[i:]
			break
		}
	}
	return func(msgLen int) (int, iter.Seq[int]) {

		// 1. Calculate total frame count
		nFrames := 0
		rem := msgLen
		for rem > 0 {
			for _, cap := range validCaps {
				if cap <= rem {
					nFrames++
					rem -= cap
					break
				}
			}
		}

		// 2. Generator iterator
		seq := func(yield func(int) bool) {
			remaining := msgLen
			for remaining > 0 {
				for _, cap := range validCaps {
					if cap <= remaining {
						if !yield(cap) {
							return
						}
						remaining -= cap
						break
					}
				}
			}
		}

		return nFrames, seq
	}
}
