/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workqueue

import (
	"container/list"
	"sync"

	"k8s.io/client-go/util/workqueue"
)

var _ workqueue.TypedInterface[any] = &TypedFair[any]{}

// TypedFair is a queue that ensures items are dequeued fairly across different
// fairness keys while maintaining FIFO order within each key.
type TypedFair[T comparable] struct {
	mu         sync.Mutex
	entries    map[string]*queueEntry[T]
	activeList *list.List
	keyFunc    FairnessKeyFunc[T]
}

// Fair is a queue that ensures items are dequeued fairly across different
// fairness keys while maintaining FIFO order within each key.
type Fair TypedFair[any]

// FairnessKeyFunc is a function that returns a string key for a given item.
// Items with different keys are dequeued fairly.
type FairnessKeyFunc[T comparable] func(T) string

// NewFair creates a new Fair instance.
func NewFair(keyFunc FairnessKeyFunc[any]) *Fair {
	return (*Fair)(NewTypedFair[any](keyFunc))
}

// NewTypedFair creates a new TypedFair instance.
func NewTypedFair[T comparable](keyFunc FairnessKeyFunc[T]) *TypedFair[T] {
	return &TypedFair[T]{
		entries:    make(map[string]*queueEntry[T]),
		activeList: list.New(),
		keyFunc:    keyFunc,
	}
}

type queueEntry[T comparable] struct {
	queue      workqueue.TypedInterface[T]
	activeElem *list.Element // Reference to the element in activeList
}

// Add inserts an item into the queue under its fairness key.
func (q *TypedFair[T]) Add(item T) {
	key := q.keyFunc(item)

	q.mu.Lock()
	defer q.mu.Unlock()

	entry, exists := q.entries[key]
	if !exists {
		entry = &queueEntry[T]{
			queue: workqueue.NewTyped[T](),
		}
		q.entries[key] = entry
	}

	entry.queue.Add(item)

	// If the queue was previously empty, add to activeList
	if entry.queue.Len() == 1 && entry.activeElem == nil {
		entry.activeElem = q.activeList.PushBack(key)
	}
}

// Get retrieves the next item from the queue, ensuring fairness across keys.
func (q *TypedFair[T]) Get() (item T, shutdown bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var elem *list.Element
	for elem = q.activeList.Front(); elem != nil; elem = elem.Next() {
		key := elem.Value.(string)
		entry := q.entries[key]

		if entry.queue.Len() == 0 {
			q.activeList.Remove(elem)
			entry.activeElem = nil
			continue
		}

		item, shutdown = entry.queue.Get()
		if shutdown {
			continue
		}

		// Check if the queue is now empty and update activeList
		if entry.queue.Len() == 0 {
			q.activeList.Remove(elem)
			entry.activeElem = nil
		} else {
			// Move to back to maintain round-robin order
			q.activeList.MoveToBack(elem)
		}

		return item, false
	}

	var zero T
	return zero, true
}

// Done marks the processing of an item as complete.
func (q *TypedFair[T]) Done(item T) {
	key := q.keyFunc(item)

	q.mu.Lock()
	defer q.mu.Unlock()

	if entry, exists := q.entries[key]; exists {
		entry.queue.Done(item)
	}
}

// Len returns the total number of items across all keys.
func (q *TypedFair[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	total := 0
	for _, entry := range q.entries {
		total += entry.queue.Len()
	}
	return total
}

// ShutDown terminates the queue and all sub-queues.
func (q *TypedFair[T]) ShutDown() {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, entry := range q.entries {
		entry.queue.ShutDown()
	}
}

// ShuttingDown checks if all sub-queues are shutting down.
func (q *TypedFair[T]) ShuttingDown() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, entry := range q.entries {
		if !entry.queue.ShuttingDown() {
			return false
		}
	}
	return true
}

// ShutDownWithDrain terminates the queue and all sub-queues, draining all.
func (q *TypedFair[T]) ShutDownWithDrain() {
	q.mu.Lock()
	defer q.mu.Unlock()

	var wg sync.WaitGroup
	for _, entry := range q.entries {
		wg.Add(1)
		go func(entry *queueEntry[T]) {
			defer wg.Done()
			entry.queue.ShutDownWithDrain()
		}(entry)
	}
	wg.Wait()
}
