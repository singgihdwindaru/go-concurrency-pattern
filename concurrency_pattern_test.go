package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"golang.org/x/sync/errgroup"
)

type Event struct{}
type Item struct {
	kind string
}

var items = map[string]Item{
	"a": {"valueA"},
	"b": {"valueB"},
	"c": {"valueC"},
	"d": {"valueD"},
}

func doSlowThing() { time.Sleep(10 * time.Millisecond) }

func consume(items ...Item) {
	for _, item := range items {
		fmt.Println(item)
	}
}

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

func FuturePatternFetchV2(name string, c chan Item) {
	// defer close(c)
	item, ok := items[name]
	doSlowThing()
	if !ok {
		close(c)
		return
	}
	c <- item
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
			name: "Do this V2",
			mock: func() {
				start := time.Now()
				items := make(chan Item, 1)
				itemC := make(chan Item, 1)

				/* Do this. The caller must set up concurrent work
				before retrieving results */
				go FuturePatternFetchV2("c", itemC)
				go FuturePatternFetchV2("b", items)
				consume(<-items, <-itemC)
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

func squares(c chan int) {
	// time.Sleep(1000 * time.Millisecond)
	for i := 0; i < 4; i++ {
		num := <-c
		fmt.Println(num * num)
	}
}
func TestSquare(t *testing.T) {
	// go clean -testcache &&  go test -run=TestSquare -v
	fmt.Println("Total goroutine ", runtime.NumGoroutine())
	c := make(chan int, 1)
	go squares(c)

	c <- 1
	c <- 2
	c <- 3
	c <- 4

	time.Sleep(time.Second)
	fmt.Println("Total goroutine ", runtime.NumGoroutine())
}

func processFile(filename string, ch chan<- string) error {
	for i := 0; i < 7; i++ {
		ch <- fmt.Sprintf("data ke : %d", i)
	}
	close(ch)
	return nil
}

func waitUntil(ctx context.Context, wg *sync.WaitGroup, until time.Time) {
	timer := time.NewTimer(time.Until(until))
	defer timer.Stop()
	<-timer.C
	wg.Done()
}

func TestDeadlock(t *testing.T) {
	// go clean -testcache &&  go test -run=TestDeadlock -v

	until := time.Now().Add(2 * time.Second)

	ch := make(chan string)
	wg := &sync.WaitGroup{}
	wg.Add(2)
	ctx := context.Background()
	go func() {
		if err := processFile("filename.txt", ch); err != nil {
			fmt.Println("Error processing file:", err)
		}
		// close(ch)
		waitUntil(ctx, wg, until)
	}()

	go func() {
		for line := range ch {
			fmt.Println(line)
		}
		waitUntil(ctx, wg, until)
	}()
	fmt.Println("Total goroutine ", runtime.NumGoroutine())
	wg.Wait()

	fmt.Println("Total goroutine ", runtime.NumGoroutine())

	fmt.Println("All goroutines finished")
	// time.Sleep(time.Second)
}

func TestChannelFuncReturnError(t *testing.T) {
	// go clean -testcache &&  go test -run=TestChannelFuncReturnError -v
	fmt.Println("Total goroutine ", runtime.NumGoroutine())

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan []string, 1)
	go ChannelFuncReturnError(ctx, ch, 0)
	go ChannelFuncReturnError(ctx, ch, 1)
	go ChannelFuncReturnError(ctx, ch, 2)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	fmt.Println(<-ch)
	fmt.Println(<-ch)
	fmt.Println(<-ch)
	fmt.Println("Total goroutine ", runtime.NumGoroutine())
}

func ChannelFuncReturnError(ctx context.Context, param chan<- []string, id int) error {
	var paymentMethodTransaction []string

	getData := func(ctx context.Context) ([]string, error) {
		data := []string{"data1", "data2", "data3"}
		return data, nil
	}
	paymentMethod, err := getData(ctx)
	if err != nil {
		return err
	}
	if id == 1 {
		time.Sleep(100 * time.Millisecond)
	}
	paymentMethodTransaction = append(paymentMethodTransaction, paymentMethod[id])

	select {
	case param <- paymentMethodTransaction:
		// return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return nil
}

func TestSelectCase(t *testing.T) {
	// go test -run=TestSelectCase -v
	fmt.Println("Total goroutine ", runtime.NumGoroutine())

	start := time.Now()
	userIDs := []string{"a", "b", "c"}

	userChan := make(chan *Item)
	errorChan := make(chan error)

	ctx := context.Background()
	wg := &sync.WaitGroup{}

	for _, userID := range userIDs {
		wg.Add(1)
		go fetchUser(ctx, userID, userChan, errorChan, wg)
	}

	for range userIDs {
		select {
		case user := <-userChan:
			wg.Add(1)
			go func() {
				defer wg.Done()
				if user.kind == "gopher" {
					time.Sleep(500 * time.Millisecond)
				}
				if user.kind == "rabbitB" {
					time.Sleep(300 * time.Millisecond)
				}
				if user.kind == "rabbitC" {
					time.Sleep(400 * time.Millisecond)
				}
				// processUser(*user)
			}()
		case err := <-errorChan:
			fmt.Println("Error occurred:", err)
		}
	}
	wg.Wait()
	fmt.Println(time.Since(start))
	fmt.Println("Total goroutine ", runtime.NumGoroutine())

}

func fetchUser(ctx context.Context, userID string, userChan chan<- *Item, errorChan chan<- error, wg *sync.WaitGroup) {
	log.Printf("%v starting fetch userID\n", userID)
	defer wg.Done()

	select {
	case <-ctx.Done():
		log.Printf("%s fetchUser got cancel %v\n", userID, time.Now().UnixMilli())
		return
	default:
	}
	// if userID == "a" {
	// 	time.Sleep(1500 * time.Millisecond)
	// }
	// if userID == "b" {
	// 	time.Sleep(1500 * time.Millisecond)
	// }
	// if userID == "c" {
	// 	time.Sleep(1500 * time.Millisecond)
	// }
	// if userID == "d" {
	// 	time.Sleep(1600 * time.Millisecond)
	// }

	log.Printf("%v searching userID\n", userID)
	item, ok := items[userID]
	if !ok {
		errorChan <- fmt.Errorf("%s not found for key %v\n", userID, time.Now().UnixMilli())
		return
	}
	select {
	case <-ctx.Done():
		log.Printf("%s fetchUser got cancel %v\n", userID, time.Now().UnixMilli())
		// errorChan <- fmt.Errorf("%s not found for key %v\n", userID, time.Now().UnixMilli())
		return
	case userChan <- &item:
	}

}

func TestCancelMultipleWorkers(t *testing.T) {
	// go clean -testcache && go test -run=TestCancelMultipleWorkers -v
	totalGoroutineStart := runtime.NumGoroutine()
	log.Printf("There are %d goroutines starting\n\n", totalGoroutineStart)

	data1 := make(chan *Item, 1)
	data2 := make(chan *Item, 1)
	data3 := make(chan *Item, 1)
	data4 := make(chan *Item, 1)
	chErr := make(chan error, 4)

	parentCtx := context.Background()
	childCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(4)
	go fetchUser(childCtx, "dd", data1, chErr, &wg)
	go fetchUser(childCtx, "a", data2, chErr, &wg)
	go fetchUser(childCtx, "b", data3, chErr, &wg)
	go fetchUser(childCtx, "cc", data4, chErr, &wg)

	var signals []bool
	var err error
	for {
		select {
		case d1, ok := <-data1:
			if ok {
				log.Printf("Received result: %v\n", d1)
			}
			signals = append(signals, true)
		case d2, ok := <-data2:
			if ok {
				log.Printf("Received result: %v\n", d2)
			}
			signals = append(signals, true)
		case d3, ok := <-data3:
			if ok {
				log.Printf("Received result: %v\n", d3)
			}
			signals = append(signals, true)
		case d4, ok := <-data4:
			if ok {
				log.Printf("Received result: %v\n", d4)
			}
			signals = append(signals, true)
		case err = <-chErr:
			// signals = append(signals, true)
			log.Printf("error occured : %v", err)
			cancel()
		}

		// Exit the loop
		if err != nil || len(signals) > 3 {
			break
		}
	}
	wg.Wait()

	// close(chErr)
	// close(data1)
	// close(data2)
	// close(data3)
	// close(data4)

	// checking goroutine leak
	actualGoroutineLeft := runtime.NumGoroutine()
	assert.Equal(t, totalGoroutineStart, actualGoroutineLeft, "goroutine leak detected : ", totalGoroutineStart)

}

func TestNonBlockingChannelOperations(t *testing.T) {
	// go test -run=TestNonBlockingChannelOperations -v
	// https://gobyexample.com/non-blocking-channel-operations
	messages := make(chan string)
	signals := make(chan bool)

	select {
	case msg := <-messages:
		fmt.Println("received message", msg)
	default:
		fmt.Println("no message received")
	}
	msg := "hi"
	/* the messages channel is unbuffered and there is no receiver,
	   the output will be "no message sent" because the send operation couldn't be performed immediately.
	   For this case,there are two ways to success the send operation for unbuffered channel :
	   1. Use separate goroutine for send operation and receiver in main goroutine
	      go func(){
	       // the second block select case
	     }()
	     <- messages // receive

	   2. use buffered channel
	   messages := make(chan string,1)
	*/
	select {
	case messages <- msg:
		fmt.Println("sent message", msg)
	default:
		fmt.Println("no message sent")
	}

	select {
	case msg := <-messages:
		fmt.Println("received message", msg)
	case sig := <-signals:
		fmt.Println("received signal", sig)
	default:
		fmt.Println("no activity")
	}
}
