package main

import (
	"container/heap"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
)

// https://pkg.go.dev/container/heap#example-package-PriorityQueue

// Item represents a generic item with a priority.
type Item[T any] struct {
	Value    T
	Priority int // Higher value means higher priority
	index    int // Index in the heap (for heap.Interface)
}

// UnboundedPriorityQueue implements heap.Interface and holds Items.
type UnboundedPriorityQueue[T any] []*Item[T]

func (pq UnboundedPriorityQueue[T]) Len() int { return len(pq) }

// We want higher priority to have a lower index in the heap for removal.
func (pq UnboundedPriorityQueue[T]) Less(i, j int) bool {
	return pq[i].Priority > pq[j].Priority
}

func (pq UnboundedPriorityQueue[T]) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *UnboundedPriorityQueue[T]) Push(x any) {
	n := len(*pq)
	item := x.(*Item[T])
	item.index = n
	*pq = append(*pq, item)
}

func (pq *UnboundedPriorityQueue[T]) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leaks
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// update modifies the priority of an Item in the queue.
func (pq *UnboundedPriorityQueue[T]) update(item *Item[T], value T, priority int) {
	item.Value = value
	item.Priority = priority
	heap.Fix(pq, item.index)
}

// https://github.com/adrianbrad/queue/blob/main/blocking.go
// https://github.com/theodesp/blockingQueues/blob/master/blockingQueue.go

// BlockingPriorityQueue wraps UnboundedPriorityQueue with thread safety.
type BlockingPriorityQueue[T any] struct {
	pq     UnboundedPriorityQueue[T] // The underlying priority queue
	mu     sync.Mutex                // Mutex for thread-safe access to the queue
	co     *sync.Cond                // Condition variable for signaling when the queue is not empty
	closed bool                      // Indicates if the queue is closed
}

// NewBlockingPriorityQueue initializes a new BlockingPriorityQueue.
func NewBlockingPriorityQueue[T any]() *BlockingPriorityQueue[T] {
	bpq := &BlockingPriorityQueue[T]{}
	// Initialize the condition variable with the mutex
	bpq.co = sync.NewCond(&bpq.mu)
	return bpq
}

// Len returns the length of the priority queue in a thread-safe manner.
func (pqw *BlockingPriorityQueue[T]) Len() int {
	pqw.mu.Lock()         // Acquire the lock to ensure thread-safe access
	defer pqw.mu.Unlock() // Release the lock when the function exits
	return pqw.pq.Len()
}

// Push adds an item to the priority queue in a thread-safe manner.
func (pqw *BlockingPriorityQueue[T]) Push(x *Item[T]) error {
	pqw.mu.Lock()         // Acquire the lock to ensure thread-safe access
	defer pqw.mu.Unlock() // Release the lock when the function exits

	if pqw.closed {
		return fmt.Errorf("queue is closed")
	}

	defer pqw.co.Signal() // Signal one waiting goroutine that an item has been added
	heap.Push(&pqw.pq, x) // Add the item to the underlying priority queue
	return nil
}

// Pop removes and returns the highest-priority item from the queue in a thread-safe manner.
func (pqw *BlockingPriorityQueue[T]) Pop() (*Item[T], error) {
	pqw.mu.Lock()         // Acquire the lock to ensure thread-safe access
	defer pqw.mu.Unlock() // Release the lock when the function exits

	// Wait until the queue is not empty or closed
	for pqw.pq.Len() <= 0 && !pqw.closed {
		pqw.co.Wait() // Release the lock and wait for a signal
	}

	if pqw.closed && pqw.pq.Len() == 0 {
		log.Debug().Msg("BlockingPriorityQueue Pop() closed")
		return nil, fmt.Errorf("queue is closed")
	}

	// Remove and return the highest-priority item
	return heap.Pop(&pqw.pq).(*Item[T]), nil
}

// Close marks the queue as closed and signals all waiting goroutines.
func (pqw *BlockingPriorityQueue[T]) Close() {
	pqw.mu.Lock()         // Acquire the lock to ensure thread-safe access
	defer pqw.mu.Unlock() // Release the lock when the function exits

	if !pqw.closed {
		pqw.closed = true
		pqw.co.Broadcast() // Wake up all waiting goroutines
		log.Debug().Msg("BlockingPriorityQueue closed")
	}
}

// https://github.com/golang-design/chann/blob/main/chann.go

// ChannelizedPriorityQueue wraps a BlockingPriorityQueue and provides in and out channels
// for interacting with the queue using a producer-consumer model.
type ChannelizedPriorityQueue[T any] struct {
	in  chan *Item[T]             // Buffered channel for incoming items
	out chan *Item[T]             // Unbuffered channel for outgoing items
	bpq *BlockingPriorityQueue[T] // Internal thread-safe priority queue
}

// NewChannelizedPriorityQueue initializes a new ChannelizedPriorityQueue.
func NewChannelizedPriorityQueue[T any]() *ChannelizedPriorityQueue[T] {
	cpq := &ChannelizedPriorityQueue[T]{
		in:  make(chan *Item[T], 16), // Buffered channel with size 16
		out: make(chan *Item[T]),     // Unbuffered channel
		bpq: NewBlockingPriorityQueue[T](),
	}

	// Start a goroutine to transfer items from the in channel to the internal queue
	go cpq.transferToQueue()

	// Start a goroutine to transfer items from the internal queue to the out channel
	go cpq.transferToOut()

	return cpq
}

// transferToQueue continuously reads from the in channel and pushes items to the internal queue.
func (cpq *ChannelizedPriorityQueue[T]) transferToQueue() {
	for item := range cpq.in {
		cpq.bpq.Push(item) // Push the item to the internal priority queue
	}
	log.Debug().Msg("ChannelizedPriorityQueue transferToQueue exited")
	// When the in channel is closed, and exhausted of all items, we can Close the bpq queue
	// transferToOut will still receive items until also exhausting the internal queue
	cpq.bpq.Close()
}

// transferToOut continuously pops items from the internal queue and sends them to the out channel.
func (cpq *ChannelizedPriorityQueue[T]) transferToOut() {
	for {
		item, err := cpq.bpq.Pop() // Pop the highest-priority item from the internal queue
		if err != nil {            // Closed
			close(cpq.out)
			log.Debug().Msg("ChannelizedPriorityQueue out channel closed")
			return
		}
		cpq.out <- item // Send the item to the out channel
	}
}

// In returns the input channel for adding items to the queue.
func (cpq *ChannelizedPriorityQueue[T]) In() chan<- *Item[T] {
	return cpq.in
}

// Out returns the output channel for consuming items from the queue.
func (cpq *ChannelizedPriorityQueue[T]) Out() <-chan *Item[T] {
	return cpq.out
}

// Close closes the in channel immediately and delays the closing of the out channel
// until all remaining items have been processed.
func (cpq *ChannelizedPriorityQueue[T]) Close() {
	// Close the in channel to stop accepting new items, transferToQueue will call bpq.Close()
	close(cpq.in)
	log.Debug().Msg("ChannelizedPriorityQueue in channel closed")

	// The out channel will be closed in transferToOut
}
