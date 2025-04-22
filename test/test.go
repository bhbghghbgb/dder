package main

import (
	"container/heap"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"
)

func main() {
	// Define test slices
	input1 := []int{5, 3, 8, 1, 7} // First slice of integers to push
	input2 := []int{4, 9, 2}       // Second slice to pop and push simultaneously

	// Test UnboundedPriorityQueue
	fmt.Println("Testing UnboundedPriorityQueue...")
	resultUnbounded := TestUnboundedPriorityQueue(input1, input2)
	fmt.Println("Popped:", resultUnbounded)
	expectedtUnbounded := slices.Equal(resultUnbounded, []int{8, 7, 5, 9, 4, 3, 2, 1})
	fmt.Println("Expected order:", expectedtUnbounded)

	// Test BlockingPriorityQueue
	fmt.Println("\nTesting BlockingPriorityQueue...")
	resultBlocking := TestBlockingPriorityQueue(input1, input2)
	fmt.Println("Popped:", resultBlocking)
	expectedBlocking := slices.Equal(resultBlocking, []int{8, 7, 5, 9, 4, 3, 2, 1})
	fmt.Println("Expected order:", expectedBlocking)

	// Test ChannelizedPriorityQueue sequentially
	fmt.Println("\nTesting TestChannelizedPriorityQueueSequential...")
	resultChannelized := TestChannelizedPriorityQueueSequential(input1, input2)
	fmt.Println("Popped:", resultChannelized)
	// first element (5) sent to out channel first, so it is received first
	expectedChannelized := slices.Equal(resultChannelized, []int{5, 8, 7, 9, 4, 3, 2, 1})
	fmt.Println("Expected order:", expectedChannelized)

	// Test ChannelizedPriorityQueue in parallel
	fmt.Println("\nTesting TestChannelizedPriorityQueueParallel...")
	resultChannelizedParallel := TestChannelizedPriorityQueueParallel(input1, input2)
	fmt.Println("Popped:", resultChannelizedParallel)
	// first element (5) sent to out channel first, so it is received first
	// can only expect first len(input2) elements to be same everytime
	expectedChannelizedParallel := slices.Equal(resultChannelizedParallel[:3], []int{5, 8, 7})
	fmt.Println("Expected order:", expectedChannelizedParallel)
	// Check if the size of the popped results matches expected
	expectedSize := len(input1)+len(input2) == len(resultChannelizedParallel)
	fmt.Println("Expected size:", expectedSize)
}

func TestUnboundedPriorityQueue(input1 []int, input2 []int) []int {
	pq := make(UnboundedPriorityQueue[int], 0)

	// Push all elements from the first input slice
	for _, value := range input1 {
		item := &Item[int]{Value: value, Priority: value}
		heap.Push(&pq, item)
	}

	// Pop len(input2) elements
	results := []int{}
	for range input2 {
		poppedItem := heap.Pop(&pq).(*Item[int])
		results = append(results, poppedItem.Value)
	}

	// Push all elements from the second input slice
	for _, value := range input2 {
		item := &Item[int]{Value: value, Priority: value}
		heap.Push(&pq, item)
	}

	// Pop all remaining elements
	for pq.Len() > 0 {
		poppedItem := heap.Pop(&pq).(*Item[int])
		results = append(results, poppedItem.Value)
	}

	return results
}

func TestBlockingPriorityQueue(input1 []int, input2 []int) []int {
	bpq := NewBlockingPriorityQueue[int]()

	// Push all elements from the first input slice
	for _, value := range input1 {
		item := &Item[int]{Value: value, Priority: value}
		bpq.Push(item)
	}

	// Pop len(input2) elements
	results := []int{}
	for range input2 {
		poppedItem, err := bpq.Pop()
		if err != nil {
			log.Println("Error popping item from BlockingPriorityQueue:", err)
			continue
		}
		results = append(results, poppedItem.Value)
	}

	// Push all elements from the second input slice
	for _, value := range input2 {
		item := &Item[int]{Value: value, Priority: value}
		bpq.Push(item)
	}

	// Pop all remaining elements
	for bpq.Len() > 0 {
		poppedItem, err := bpq.Pop()
		if err != nil {
			log.Println("Error popping item from BlockingPriorityQueue:", err)
			continue
		}
		results = append(results, poppedItem.Value)
	}

	return results
}

func TestChannelizedPriorityQueueSequential(input1 []int, input2 []int) []int {
	cpq := NewChannelizedPriorityQueue[int]()

	// Push all elements from the first input slice
	for _, value := range input1 {
		item := &Item[int]{Value: value, Priority: value}
		cpq.In() <- item
	}

	// Pop len(input2) elements
	results := []int{}
	for range input2 {
		poppedItem := <-cpq.Out()
		results = append(results, poppedItem.Value)
	}

	// Push all elements from the second input slice
	for _, value := range input2 {
		item := &Item[int]{Value: value, Priority: value}
		cpq.In() <- item
	}

	cpq.Close()

	// Wait for the internal goroutines to transfer all elements to the queue
	time.Sleep(time.Second)

	// Pop all remaining elements
	for item := range cpq.Out() {
		results = append(results, item.Value)
	}

	return results
}

func TestChannelizedPriorityQueueParallel(input1 []int, input2 []int) []int {
	cpq := NewChannelizedPriorityQueue[int]()

	// Push all elements from the first input slice
	for _, value := range input1 {
		item := &Item[int]{Value: value, Priority: value}
		cpq.In() <- item
	}

	// Pop len(input2) elements
	results := []int{}
	for range input2 {
		poppedItem := <-cpq.Out()
		results = append(results, poppedItem.Value)
	}

	// Push all elements from the second input slice
	for _, value := range input2 {
		item := &Item[int]{Value: value, Priority: value}
		cpq.In() <- item
	}

	cpq.Close()

	var wg sync.WaitGroup
	var mu sync.Mutex // Mutex for thread-safe appending to the results slice

	// Spawn four workers
	numWorkers := 4
	wg.Add(numWorkers)
	for range numWorkers {
		go func() {
			defer wg.Done()
			for item := range cpq.Out() {
				mu.Lock()
				results = append(results, item.Value)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	return results
}

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
	}
}

// https://github.com/golang-design/chann/blob/main/chann.go

// ChannelizedPriorityQueue wraps a BlockingPriorityQueue and provides in and out channels
// for interacting with the queue using a producer-consumer model.
type ChannelizedPriorityQueue[T any] struct {
	in  chan *Item[T]             // Channel for incoming items
	out chan *Item[T]             // Channel for outgoing items
	bpq *BlockingPriorityQueue[T] // Internal thread-safe priority queue
}

// NewChannelizedPriorityQueue initializes a new ChannelizedPriorityQueue.
func NewChannelizedPriorityQueue[T any]() *ChannelizedPriorityQueue[T] {
	cpq := &ChannelizedPriorityQueue[T]{
		in:  make(chan *Item[T]),
		out: make(chan *Item[T]),
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

	// The out channel will be closed in transferToOut
}
