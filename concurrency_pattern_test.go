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
func TestSingleElementBufferedChannel2(t *testing.T) {
	/*
		go test -run=TestSingleElementBufferedChannel2 -v
		NOTE : Notice the 10ms difference of timer result from the tests
		1. If we return without waiting for the futures to complete, how long will they continue using resources?
		2. What happens in case of cancellation or error? if so, what happens if we cancel it and then try to read from the channel?
		Will we receive a zero-value, some other sentinel value, or block?

	*/
	fmt.Println("Remaining goroutines:", runtime.NumGoroutine())

	tests := []struct {
		name string
		mock func()
	}{
		{
			name: "Do this V2",
			mock: func() {
				start := time.Now()
				items := make(chan Item)

				/* Do this. The caller must set up concurrent work
				before retrieving results */
				go FuturePatternFetchV2("a", items)
				go FuturePatternFetchV2("b", items)
				consume(<-items, <-items)
				close(items)
				fmt.Println(time.Since(start))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mock()
		})
	}
	fmt.Println("Remaining goroutines:", runtime.NumGoroutine()) //-1 for the function
	// fmt.Println("Thread : ", runtime.GOMAXPROCS(-1))               //-1 for the function
	// fmt.Println("CPU :", runtime.NumCPU())

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

func TestChannel(t *testing.T) {
	// go clean -testcache && go test ./src/order/usecase -run=TestChannel -v
	// go test -run=TestChannel -v
	start := time.Now()
	errChan1 := make(chan error, 1)
	errChan2 := make(chan error, 1)
	go func() {
		errChan1 <- errors.New("error chan 1")
		time.Sleep(10 * time.Millisecond)
		// close(errChan1)
	}()
	go func() {
		errChan2 <- errors.New("error chan 2")
		time.Sleep(10 * time.Millisecond)
		// close(errChan2)
	}()
	err1 := <-errChan1
	err2 := <-errChan2
	fmt.Println(err1)
	fmt.Println(err2)
	elapsed := time.Since(start)
	fmt.Println(elapsed)
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

func TestBuff(t *testing.T) {
	fmt.Println("Total goroutine ", runtime.NumGoroutine())
	// ctx := context.Background().Done()
	// go clean -testcache &&  go test -run=TestBuff -v
	// g1
	ch := make(chan int, 1)
	// defer close(ch)
	go func() {
		// g2
		heavyProccess := func() { time.Sleep(2000 * time.Millisecond) }
		heavyProccess()
		fmt.Println("hello goroutine number 2, ", <-ch)
		// close(ch)
		// fmt.Println("hello goroutine number 2, ? ")
		// fmt.Println("hello goroutine number 2, ? ")
		// ch <- 2
		// ch <- 4
		// ch <- 1
		// ch <- 3
		// close(ch)
	}()
	time.Sleep(1000 * time.Millisecond)
	fmt.Println("hello goroutine number 1")
	var err error
	// err = errors.New("some error")
	if err != nil { // wait a second for GC release goroutine
		fmt.Println(err)

		time.Sleep(500 * time.Millisecond)
		fmt.Println("Total goroutine ", runtime.NumGoroutine())
		return
	}
	ch <- 2
	// fmt.Println(<-ch)
	// fmt.Println(<-ch)
	// fmt.Println(<-ch)
	fmt.Println("finish goroutine number 1")
	// fmt.Println(<-ch)

	time.Sleep(100 * time.Millisecond) // wait a second for GC release goroutine
	fmt.Println("Total goroutine ", runtime.NumGoroutine())
}

func TestBussff2(t *testing.T) {
	fmt.Println("Total goroutine ", runtime.NumGoroutine())

	// go clean -testcache &&  go test -run=TestBuff -v
	// g1
	ch := make(chan int, 1)
	go func() {
		// g2
		heavyProccess := func() { time.Sleep(1000 * time.Millisecond) }
		heavyProccess()
		fmt.Println("hello goroutine number 2")
		ch <- 2
		ch <- 4
		ch <- 1
		ch <- 3
		close(ch)
	}()

	time.Sleep(2000 * time.Millisecond)
	fmt.Println("hello goroutine number 1")
	// g2
	fmt.Println(<-ch)
	fmt.Println(<-ch)
	fmt.Println(<-ch)
	fmt.Println(<-ch)
	fmt.Println("finish goroutine number 1")
	// fmt.Println(<-ch)

	time.Sleep(1 * time.Millisecond) // wait a second for GC release goroutine
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

func fetchUser(ctx context.Context, userID string, userChan chan<- *Item, errorChan chan error, wg *sync.WaitGroup) {
	defer wg.Done()

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
	log.Printf("%v starting fetch userID\n", userID)

	log.Printf("%v searching userID\n", userID)
	item, ok := items[userID]
	if !ok {
		errorChan <- fmt.Errorf("%s not found for key %v\n", userID, time.Now().UnixMilli())
		// userChan <- &Item{}
		return
	}

	select {
	case <-ctx.Done():
		log.Printf("%s fetchUser got cancel %v\n", userID, time.Now().UnixMilli())
		close(userChan)
	case userChan <- &item:
		log.Printf("%v Finish userID %v\n", &item, time.Now().UnixMilli())
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
	chErr := make(chan error)

	parentCtx := context.Background()
	childCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	wg := &sync.WaitGroup{}
	wg.Add(4)

	go fetchUser(childCtx, "dd", data1, chErr, wg)
	go fetchUser(childCtx, "aa", data2, chErr, wg)
	go fetchUser(childCtx, "b", data3, chErr, wg)
	go fetchUser(childCtx, "c", data4, chErr, wg)

	go func() {
		wg.Wait()
		close(chErr)
		close(data1)
		close(data2)
		close(data3)
		close(data4)
	}()

	var done []bool
	var isError bool

	for len(done) < 4 && isError == false {
		select {
		case d1 := <-data1:
			log.Println("the value ", *d1)
			done = append(done, true)
		case d2 := <-data2:
			log.Println("the value ", *d2)
			done = append(done, true)
		case d3 := <-data3:
			log.Println("the value ", *d3)
			done = append(done, true)
		case d4 := <-data4:
			log.Println("the value ", *d4)
			done = append(done, true)
		case err := <-chErr:
			cancel()
			isError = true
			log.Printf("error occured : %v", err)
			return
		}
	}

	log.Println("waiting ...")
	wg.Wait()

	fmt.Println()
	// checking goroutine leak
	actualGoroutineLeft := runtime.NumGoroutine()
	assert.Equal(t, totalGoroutineStart, actualGoroutineLeft, "goroutine leak detected")
}

func TestNonBlocking(t *testing.T) {
	// go test -run=TestNonBlocking -v

	messages := make(chan string)
	signals := make(chan bool)

	select {
	case msg := <-messages:
		fmt.Println("received message", msg)
	default:
		fmt.Println("no message received")
	}
	msg := "hi"

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
