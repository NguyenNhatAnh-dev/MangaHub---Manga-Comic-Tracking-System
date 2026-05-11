package protocol

import (
	"sync"

	"github.com/mangahub/mangahub/pkg/models"
)

type Broker struct {
	mu             sync.RWMutex
	progressSubs   map[chan models.ProgressUpdate]struct{}
	notifySubs     map[chan models.Notification]struct{}
}

var defaultBroker = &Broker{
	progressSubs: make(map[chan models.ProgressUpdate]struct{}),
	notifySubs:   make(map[chan models.Notification]struct{}),
}

func Default() *Broker {
	return defaultBroker
}

func (b *Broker) SubscribeProgress() chan models.ProgressUpdate {
	ch := make(chan models.ProgressUpdate, 64)
	b.mu.Lock()
	b.progressSubs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broker) UnsubscribeProgress(ch chan models.ProgressUpdate) {
	b.mu.Lock()
	delete(b.progressSubs, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *Broker) PublishProgress(p models.ProgressUpdate) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.progressSubs {
		select {
		case ch <- p:
		default:
		}
	}
}

func (b *Broker) SubscribeNotification() chan models.Notification {
	ch := make(chan models.Notification, 64)
	b.mu.Lock()
	b.notifySubs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broker) UnsubscribeNotification(ch chan models.Notification) {
	b.mu.Lock()
	delete(b.notifySubs, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *Broker) PublishNotification(n models.Notification) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.notifySubs {
		select {
		case ch <- n:
		default:
		}
	}
}
