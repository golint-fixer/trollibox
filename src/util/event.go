package util

import (
	"sync"
	"time"
)

type Emitter struct {
	listeners map[chan string]struct{}
	lock      sync.Mutex

	release      time.Duration
	releaseReset map[string]chan struct{}
}

// Creates new emitter. The release attribute determines how much time the
// event should be buffered to prevent the emission of duplicate events.
func NewEmitter(release time.Duration) *Emitter {
	return &Emitter{
		listeners:    map[chan string]struct{}{},
		releaseReset: map[string]chan struct{}{},
		release:      release,
	}
}

func (emitter *Emitter) Emit(event string) {
	emitter.lock.Lock()
	defer emitter.lock.Unlock()

	if emitter.release == 0 {
		for l := range emitter.listeners {
			l <- event
		}
		return
	}

	// Check wether the event is scheduled for emission and clear it.
	if reset, ok := emitter.releaseReset[event]; ok {
		reset <- struct{}{}
		return
	}

	go func() {
		emitter.lock.Lock()
		reset := make(chan struct{})
		emitter.releaseReset[event] = reset
		emitter.lock.Unlock()

	loop:
		for {
			select {
			case <-time.After(emitter.release):
				emitter.lock.Lock()
				for l := range emitter.listeners {
					l <- event
				}
				emitter.lock.Unlock()
				break loop
			case <-reset:
			}
		}

		emitter.lock.Lock()
		close(reset)
		delete(emitter.releaseReset, event)
		emitter.lock.Unlock()
	}()
}

func (emitter *Emitter) Listen() chan string {
	emitter.lock.Lock()
	defer emitter.lock.Unlock()

	ch := make(chan string, 16)
	emitter.listeners[ch] = struct{}{}
	return ch
}

func (emitter *Emitter) Unlisten(ch chan string) {
	emitter.lock.Lock()
	defer emitter.lock.Unlock()

	close(ch)
	delete(emitter.listeners, ch)
}
