package main

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
)

// Condition variable page 37
type ItemCondition = int

type Queue struct {
	mu        sync.Mutex
	items     []ItemCondition
	itemAdded sync.Cond
}

func NewQueue() *Queue {
	q := new(Queue)
	q.itemAdded.L = &q.mu
	return q
}

func (q *Queue) Get() ItemCondition {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) == 0 {
		q.itemAdded.Wait()
	}
	ItemCondition := q.items[0]
	q.items = q.items[1:]
	return ItemCondition
}

func (q *Queue) Put(item ItemCondition) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
	q.itemAdded.Signal()
}

func (q *Queue) GetMany(n int) []ItemCondition {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) < n {
		q.itemAdded.Wait()
	}
	items := q.items[:n:n]
	q.items = q.items[n:]
	return items
}

func randYield() {
	if rand.Intn(3) == 0 {
		runtime.Gosched()
	}
}

func TestWaitAndSignal(t *testing.T) {
	/*
		go test -run=TestConditionalVariable -v
	*/
	q := NewQueue()

	var wg sync.WaitGroup
	for n := 10; n > 0; n-- {
		wg.Add(1)
		go func(n int) {
			randYield()
			items := q.GetMany(n)
			fmt.Printf("%2d: %2d\n", n, items)
			wg.Done()
		}(n)
	}

	for i := 0; i < 100; i++ {
		q.Put(i)
		runtime.Gosched()
	}

	wg.Wait()
}
