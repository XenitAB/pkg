package channels

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMerge(t *testing.T) {
	defer goleak.VerifyNone(t)
	firstCh := make(chan string, 2)
	firstCh <- "hello"
	firstCh <- "world"
	secondCh := make(chan string)
	mergeCh := Merge(firstCh, secondCh)
	close(firstCh)
	close(secondCh)
	for range mergeCh {
	}

}
