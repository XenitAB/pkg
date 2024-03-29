package channels

import (
	"sync"
)

func Merge[T any](cs ...<-chan T) <-chan T {
	var wg sync.WaitGroup
	out := make(chan T)

	output := func(c <-chan T) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func Map[T any, U any](in <-chan T, mapFunc func(T) U) <-chan U {
	out := make(chan U)
	go func() {
		for v := range in {
			out <- mapFunc(v)
		}
	}()
	return out
}
