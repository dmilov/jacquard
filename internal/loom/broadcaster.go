package loom

import "sync"

// Broadcaster fans out byte slices to all subscribed channels.
type Broadcaster struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
	closed  bool
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{clients: make(map[chan []byte]struct{})}
}

func (b *Broadcaster) Subscribe() chan []byte {
	ch := make(chan []byte, 256)
	b.mu.Lock()
	if !b.closed {
		b.clients[ch] = struct{}{}
	}
	b.mu.Unlock()
	return ch
}

func (b *Broadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *Broadcaster) Send(data []byte) {
	cp := make([]byte, len(data))
	copy(cp, data)
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- cp:
		default: // drop if consumer is slow
		}
	}
}

func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.clients {
		close(ch)
		delete(b.clients, ch)
	}
}
