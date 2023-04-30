package main

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type Event struct{}
type Item struct {
	kind string
}

var items = map[string]Item{
	"a": {"gopher"},
	"b": {"rabbit"},
}

func doSlowThing() { time.Sleep(10 * time.Millisecond) }

// This is not how we write Go. (You likely know that already.)
// CallbackPatternFetch immediately returns, then fetches the item and
// invokes f in a goroutine when the item is available.
// If the item does not exist,
// Fetch invokes f on the zero Item.
func CallbackPatternFetch(name string, f func(Item)) {
	go func() {
		item := items[name]
		doSlowThing()
		f(item)
	}()
}
func TestCallbackPattern(t *testing.T) {
	/*
		go test -run=TestCallbackPattern -v
	*/
	start := time.Now()

	n := int32(0)
	CallbackPatternFetch("a", func(i Item) {
		fmt.Println(i)
		if atomic.AddInt32(&n, 1) == 2 {
			fmt.Println(time.Since(start))
		}
	})
	CallbackPatternFetch("b", func(i Item) {
		fmt.Println(i)
		if atomic.AddInt32(&n, 1) == 2 {
			fmt.Println(time.Since(start))
		}
	})
	time.Sleep(1 * time.Second)
	// select {}
}

// The Go analogue to a Future is a single-element buffered channel.
// FuturePatternFetch immediately returns a channel, then fetches
// the requested item and sends it on the channel.
// If the item does not exist,
// Fetch closes the channel without sending.
func FuturePatternFetch(name string) <-chan Item {
	c := make(chan Item, 1)
	go func() {
		item, ok := items[name]
		doSlowThing()
		if !ok {
			close(c)
			return
		}
		c <- item
	}()

	return c
}

func consume(a, b Item) {
	fmt.Println(a, b)
}
func TestSingleElementBufferedChannel(t *testing.T) {
	/*
		go test -run=TestSingleElementBufferedChannel -v
		NOTE : Notice the 10ms difference of timer result from the tests
	*/
	tests := []struct {
		name string
		mock func()
	}{
		{
			name: "Do this",
			mock: func() {
				start := time.Now()
				/* Do this. The caller must set up concurrent work
				before retrieving results */
				a := FuturePatternFetch("a")
				b := FuturePatternFetch("b")
				consume(<-a, <-b)
				fmt.Println(time.Since(start))
			},
		},
		{
			name: "Don't do this",
			mock: func() {
				start := time.Now()
				/* Don't do this. If they retrieve the results too early,
				the program executes sequentially (blocking) instead of
				concurrently. */
				a := <-FuturePatternFetch("a")
				b := <-FuturePatternFetch("b")
				consume(a, b)
				fmt.Println(time.Since(start))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mock()
		})
	}
}

// A channel fed by one goroutine and read by another acts as a queue.
// Glob finds all items with names matching pattern
// and sends them on the returned channel.
// It closes the channel when all items have been sent.
func Glob(pattern string) <-chan Item {
	c := make(chan Item)
	go func() {
		defer close(c)
		for name, item := range items {
			if ok, _ := filepath.Match(pattern, name); !ok {
				continue
			}
			c <- item
		}
	}()
	return c
}

func TestQueue(t *testing.T) {
	/*
		go test -run=TestQueue -v
	*/
	for item := range Glob("[ab]*") {
		fmt.Println(item)
	}
}
