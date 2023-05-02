package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
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

func consume(a, b Item) { fmt.Println(a, b) }

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

func fetch(ctx context.Context, name string) (Item, error) {
	item, ok := items[name]
	doSlowThing()
	if !ok {
		errMsg := fmt.Sprint("item not found ", name)
		return Item{}, errors.New(errMsg)
	}
	return item, nil
}

func TestAsyncCallerSite(t *testing.T) {
	/*
		go test -run=TestAsyncCallerSite -v
	*/
	start := time.Now()

	var a, b Item
	c := context.Background()
	g, ctx := errgroup.WithContext(c)
	g.Go(func() (err error) {
		a, err = fetch(ctx, "a")
		return err
	})
	g.Go(func() (err error) {
		b, err = fetch(ctx, "b")
		return err
	})

	err := g.Wait()
	if err != nil {
		fmt.Println(err)
	}
	consume(a, b)
	fmt.Println(time.Since(start))

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

func TestSingleElementBufferedChannel(t *testing.T) {
	/*
		go test -run=TestSingleElementBufferedChannel -v
		NOTE : Notice the 10ms difference of timer result from the tests
		1. If we return without waiting for the futures to complete, how long will they continue using resources?
		2. What happens in case of cancellation or error? if so, what happens if we cancel it and then try to read from the channel?
		Will we receive a zero-value, some other sentinel value, or block?

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

func TestProducerWithoutConsumer(t *testing.T) {
	/*
		go test -run=TestProducerWithoutConsumer -v
	*/
	c := make(chan int)

	go func() {
		defer close(c)
		c <- 8
	}()
	// time.Sleep(1 * time.Second)
	// <-c
	fmt.Println("processeing ...... ")
	fmt.Println("Done ")
}

func printResult(done chan string) {
	fmt.Println("2")
	done <- "3"
	done <- "10"
	fmt.Println("4")
	done <- "5"
	fmt.Println("6")
}
func TestConc(t *testing.T) {
	/*
		go test -run=TestConc -v
	*/
	fmt.Println("1")
	done := make(chan string)
	go printResult(done)
	fmt.Println("7")
	fmt.Println(<-done)
	fmt.Println("8")
	fmt.Println(<-done)
	fmt.Println("9")
	fmt.Println(<-done)
}
